package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/hawawa4/breadbotdiscord/internal/db"
)

// messageDTO is the JSON shape for a message row.
//
// Discord snowflake IDs are serialized as JSON strings (the `,string` tag), not
// numbers: real snowflakes exceed JavaScript's Number.MAX_SAFE_INTEGER (2^53),
// so a JS/browser client parsing them as numbers would silently corrupt them.
type messageDTO struct {
	OgMessageID         int64              `json:"ogmessage_id,string"`
	ReplyMessageJumpURL string             `json:"replymessage_jump_url"`
	ReplyMessageID      int64              `json:"replymessage_id,string"`
	AuthorID            int64              `json:"author_id,string"`
	ChannelID           int64              `json:"channel_id,string"`
	GuildID             int64              `json:"guild_id,string"`
	Roundness           *float64           `json:"roundness"`
	Labels              map[string]float64 `json:"labels"`
	// AnnotatedImage is the basename of the annotated prediction PNG, or null if
	// none was produced/persisted for this message. The client builds the URL as
	// <base>/api/images/predictions/<AnnotatedImage>.
	AnnotatedImage *string `json:"annotated_image"`
}

func toMessageDTO(m db.Message) messageDTO {
	dto := messageDTO{
		OgMessageID:         m.OgMessageID,
		ReplyMessageJumpURL: m.ReplyMessageJumpURL,
		ReplyMessageID:      m.ReplyMessageID,
		AuthorID:            m.AuthorID,
		ChannelID:           m.ChannelID,
		GuildID:             m.GuildID,
		Labels:              m.Labels,
	}
	if m.Roundness.Valid {
		r := m.Roundness.Float64
		dto.Roundness = &r
	}
	if m.ImageFilename.Valid && m.ImageFilename.String != "" {
		f := m.ImageFilename.String
		dto.AnnotatedImage = &f
	}
	return dto
}

// handleHealthz reports liveness: DB reachable + Discord session ready.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	dbOK := s.db.Ping() == nil
	discordOK := s.bot != nil && s.bot.Ready()

	status := http.StatusOK
	if !dbOK || !discordOK {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"status":  statusText(dbOK && discordOK),
		"db":      dbOK,
		"discord": discordOK,
	})
}

// handleLeaderboard returns best/worst roundness rows.
// Query params: order=max|min (default max), n (default 10, clamped 1..100).
func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	order := r.URL.Query().Get("order")
	if order == "" {
		order = "max"
	}
	if order != "max" && order != "min" {
		writeError(w, http.StatusBadRequest, "order must be 'max' or 'min'")
		return
	}

	n := 10
	if v := r.URL.Query().Get("n"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "n must be an integer")
			return
		}
		n = parsed
	}
	if n < 1 {
		n = 1
	}
	if n > 100 {
		n = 100
	}

	var (
		rows []db.Message
		err  error
	)
	if order == "max" {
		rows, err = s.db.GetMaxRoundnessLeaderboard(n)
	} else {
		rows, err = s.db.GetMinRoundnessLeaderboard(n)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	out := make([]messageDTO, len(rows))
	for i, m := range rows {
		out[i] = toMessageDTO(m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": order, "n": n, "rows": out})
}

// handleUserRoundness returns a user's min/max roundness + last-50 history.
func (s *Server) handleUserRoundness(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}

	// author_id as a string: snowflakes exceed JS's safe integer range.
	resp := map[string]any{"author_id": strconv.FormatInt(id, 10)}

	if m, err := s.db.GetMinRoundnessForUser(id); err == nil {
		resp["min"] = toMessageDTO(m)
	} else if !errors.Is(err, db.ErrUserNotFound) {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if m, err := s.db.GetMaxRoundnessForUser(id); err == nil {
		resp["max"] = toMessageDTO(m)
	} else if !errors.Is(err, db.ErrUserNotFound) {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	history, err := s.db.GetRoundnessHistory(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	points := make([]map[string]any, len(history))
	for i, p := range history {
		points[i] = map[string]any{"index": p.Index, "roundness": p.Roundness}
	}
	resp["history"] = points

	writeJSON(w, http.StatusOK, resp)
}

// handleUser returns cached user info.
func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	u, err := s.db.SelectUser(id)
	if errors.Is(err, db.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	var nick *string
	if u.AuthorNickname.Valid {
		n := u.AuthorNickname.String
		nick = &n
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"author_id":       strconv.FormatInt(u.AuthorID, 10),
		"author_name":     u.AuthorName,
		"author_nickname": nick,
	})
}

// handleMessage returns a single message's stats.
func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "ogmessage_id")
	if !ok {
		return
	}
	m, err := s.db.GetMessage(id)
	if errors.Is(err, db.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, toMessageDTO(m))
}

// handleStatsSummary returns server-wide aggregate stats for the dashboard.
func (s *Server) handleStatsSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.db.GetStatsSummary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"scored_count":   summary.ScoredCount,
		"distinct_users": summary.DistinctUsers,
		"avg_roundness":  summary.AvgRoundness,
		"max_roundness":  summary.MaxRoundness,
	})
}

// userDTO is the JSON shape for a discord user. author_id is a string for the
// same snowflake-precision reason as messageDTO.
type userDTO struct {
	AuthorID       int64   `json:"author_id,string"`
	AuthorName     string  `json:"author_name"`
	AuthorNickname *string `json:"author_nickname"`
}

func toUserDTO(u db.User) userDTO {
	dto := userDTO{AuthorID: u.AuthorID, AuthorName: u.AuthorName}
	if u.AuthorNickname.Valid {
		n := u.AuthorNickname.String
		dto.AuthorNickname = &n
	}
	return dto
}

// handleUsers returns a paginated user directory.
// Query params: limit (default 50, clamped 1..200), offset (default 0).
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		limit = parsed
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "offset must be an integer")
			return
		}
		if parsed > 0 {
			offset = parsed
		}
	}

	users, err := s.db.ListUsers(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	total, err := s.db.CountUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	out := make([]userDTO, len(users))
	for i, u := range users {
		out[i] = toUserDTO(u)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"limit":  limit,
		"offset": offset,
		"total":  total,
		"rows":   out,
	})
}

// pathID parses an int64 path parameter, writing a 400 and returning false on
// failure.
func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func statusText(ok bool) string {
	if ok {
		return "ok"
	}
	return "degraded"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
