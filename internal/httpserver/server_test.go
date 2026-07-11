package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hawawa4/breadbotdiscord/internal/db"
)

// knownRoundnessUser and roundest og id come from the committed fixture.
const (
	knownUser    = int64(95618667529637888)
	roundestOgID = int64(1419411009986367640)
)

type fakeBot struct{ ready bool }

func (f fakeBot) Ready() bool { return f.ready }

func newTestServer(t *testing.T, botReady bool) *Server {
	t.Helper()
	src := filepath.Join("..", "..", "dbdata", "messages.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer in.Close()
	dst := filepath.Join(t.TempDir(), "messages.db")
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy db: %v", err)
	}
	out.Close()

	database, err := db.Open(dst)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	// A downloads dir with a predictions/ fixture image so image-route tests
	// have something to serve.
	downloads := t.TempDir()
	predDir := filepath.Join(downloads, "predictions")
	if err := os.MkdirAll(predDir, 0o755); err != nil {
		t.Fatalf("mkdir predictions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(predDir, "loaf.png"), []byte("PNGDATA"), 0o644); err != nil {
		t.Fatalf("write fixture image: %v", err)
	}

	return New(":0", database, fakeBot{ready: botReady}, "", "", downloads)
}

func doGET(t *testing.T, s *Server, path string) (*http.Response, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	s.routes().ServeHTTP(rec, req)
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	return res, body
}

func TestHealthzReady(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/healthz")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", res.StatusCode, body)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["db"] != true || out["discord"] != true || out["status"] != "ok" {
		t.Errorf("unexpected health: %v", out)
	}
}

func TestHealthzDiscordNotReady(t *testing.T) {
	s := newTestServer(t, false)
	res, body := doGET(t, s, "/healthz")
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["status"] != "degraded" || out["discord"] != false {
		t.Errorf("unexpected health: %v", out)
	}
}

func TestLeaderboard(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/leaderboard?order=max&n=3")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	var out struct {
		Order string       `json:"order"`
		N     int          `json:"n"`
		Rows  []messageDTO `json:"rows"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Order != "max" || out.N != 3 || len(out.Rows) != 3 {
		t.Fatalf("got order=%s n=%d rows=%d", out.Order, out.N, len(out.Rows))
	}
	// Descending by roundness.
	for i := 1; i < len(out.Rows); i++ {
		if out.Rows[i-1].Roundness == nil || out.Rows[i].Roundness == nil {
			t.Fatal("roundness should be non-nil in leaderboard")
		}
		if *out.Rows[i-1].Roundness < *out.Rows[i].Roundness {
			t.Error("not descending")
		}
	}
}

func TestLeaderboardBadOrder(t *testing.T) {
	s := newTestServer(t, true)
	res, _ := doGET(t, s, "/api/leaderboard?order=sideways")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
}

func TestLeaderboardClampN(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/leaderboard?n=9999")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var out struct {
		N int `json:"n"`
	}
	json.Unmarshal(body, &out)
	if out.N != 100 {
		t.Errorf("n = %d, want clamped to 100", out.N)
	}
}

func TestUserRoundness(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/users/95618667529637888/roundness")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	var out struct {
		AuthorID string           `json:"author_id"`
		Min      *messageDTO      `json:"min"`
		Max      *messageDTO      `json:"max"`
		History  []map[string]any `json:"history"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AuthorID != strconv.FormatInt(knownUser, 10) {
		t.Errorf("author_id = %q, want %q", out.AuthorID, strconv.FormatInt(knownUser, 10))
	}
	if out.Min == nil || out.Max == nil {
		t.Fatal("min/max should be present for known user")
	}
	if len(out.History) != 10 {
		t.Errorf("history len = %d, want 10", len(out.History))
	}
}

func TestUserRoundnessUnknownUser(t *testing.T) {
	// A user with no rows still returns 200 with empty min/max and history.
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/users/1/roundness")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var out struct {
		Min     *messageDTO      `json:"min"`
		History []map[string]any `json:"history"`
	}
	json.Unmarshal(body, &out)
	if out.Min != nil {
		t.Error("expected no min for unknown user")
	}
	if len(out.History) != 0 {
		t.Errorf("history len = %d, want 0", len(out.History))
	}
}

func TestGetMessage(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/messages/1419411009986367640")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	var out messageDTO
	json.Unmarshal(body, &out)
	if out.OgMessageID != roundestOgID {
		t.Errorf("og id = %d, want %d", out.OgMessageID, roundestOgID)
	}
	if out.Roundness == nil {
		t.Error("roundness should be present")
	}
	if out.Labels["bread"] <= 0 {
		t.Error("expected positive bread label")
	}
}

func TestGetMessageNotFound(t *testing.T) {
	s := newTestServer(t, true)
	res, _ := doGET(t, s, "/api/messages/999999")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestGetMessageBadID(t *testing.T) {
	s := newTestServer(t, true)
	res, _ := doGET(t, s, "/api/messages/not-a-number")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
}

// TestIDsSerializedAsStrings guards the snowflake-precision fix: IDs must come
// over the wire as JSON strings, and a full 64-bit snowflake must survive
// round-trip without the precision loss a JSON number would cause in JS.
func TestIDsSerializedAsStrings(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/messages/1419411009986367640")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	// Decode into a raw map: the ID fields must be JSON strings, not numbers.
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := raw["ogmessage_id"].(string)
	if !ok {
		t.Fatalf("ogmessage_id is %T, want string", raw["ogmessage_id"])
	}
	if got != "1419411009986367640" {
		t.Errorf("ogmessage_id = %q, want exact snowflake string", got)
	}
	// And it still decodes back into the typed DTO via the ,string tag.
	var dto messageDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("decode dto: %v", err)
	}
	if dto.OgMessageID != roundestOgID {
		t.Errorf("og id = %d, want %d", dto.OgMessageID, roundestOgID)
	}
}

// TestBasePathRouting verifies that when mounted under a prefix, routes answer
// under that prefix, the bare unprefixed path 404s, and healthz stays reachable
// at the root for infra checks.
func TestBasePathRouting(t *testing.T) {
	s := newTestServer(t, true)
	s.basePath = "/breadbot"
	h := s.handler()

	do := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}

	if code := do("/breadbot/api/leaderboard"); code != http.StatusOK {
		t.Errorf("prefixed leaderboard = %d, want 200", code)
	}
	if code := do("/api/leaderboard"); code != http.StatusNotFound {
		t.Errorf("unprefixed leaderboard = %d, want 404", code)
	}
	if code := do("/healthz"); code != http.StatusOK {
		t.Errorf("root healthz = %d, want 200", code)
	}
	if code := do("/breadbot/healthz"); code != http.StatusOK {
		t.Errorf("prefixed healthz = %d, want 200", code)
	}
}

func TestStatsSummary(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/stats/summary")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	var out struct {
		ScoredCount   int     `json:"scored_count"`
		DistinctUsers int     `json:"distinct_users"`
		AvgRoundness  float64 `json:"avg_roundness"`
		MaxRoundness  float64 `json:"max_roundness"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ScoredCount <= 0 || out.DistinctUsers <= 0 {
		t.Errorf("expected positive counts, got %+v", out)
	}
	if out.MaxRoundness <= 0 || out.MaxRoundness > 1 {
		t.Errorf("max_roundness = %v, want (0,1]", out.MaxRoundness)
	}
	if out.AvgRoundness <= 0 || out.AvgRoundness > out.MaxRoundness {
		t.Errorf("avg_roundness = %v, want (0, max]", out.AvgRoundness)
	}
}

func TestUsersList(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/users?limit=2")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	var out struct {
		Limit  int       `json:"limit"`
		Offset int       `json:"offset"`
		Total  int       `json:"total"`
		Rows   []userDTO `json:"rows"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Limit != 2 {
		t.Errorf("limit = %d, want 2", out.Limit)
	}
	if len(out.Rows) > 2 {
		t.Errorf("rows = %d, want <= 2", len(out.Rows))
	}
	if out.Total < len(out.Rows) {
		t.Errorf("total %d < rows %d", out.Total, len(out.Rows))
	}
	// author_id must be a string in JSON (snowflake precision).
	var raw map[string]any
	json.Unmarshal(body, &raw)
	if rows, ok := raw["rows"].([]any); ok && len(rows) > 0 {
		row := rows[0].(map[string]any)
		if _, ok := row["author_id"].(string); !ok {
			t.Errorf("author_id is %T, want string", row["author_id"])
		}
	}
}

func TestUsersListClampLimit(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/users?limit=9999")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var out struct {
		Limit int `json:"limit"`
	}
	json.Unmarshal(body, &out)
	if out.Limit != 200 {
		t.Errorf("limit = %d, want clamped to 200", out.Limit)
	}
}

func TestImageServed(t *testing.T) {
	s := newTestServer(t, true)
	res, body := doGET(t, s, "/api/images/predictions/loaf.png")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	if string(body) != "PNGDATA" {
		t.Errorf("body = %q, want fixture content", body)
	}
}

func TestImageNotFound(t *testing.T) {
	s := newTestServer(t, true)
	res, _ := doGET(t, s, "/api/images/predictions/missing.png")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestImageTraversalRejected(t *testing.T) {
	// Encoded and literal traversal attempts must never escape the directory.
	// The mux cleans some, safeImageName rejects the rest; either way, not 200.
	for _, name := range []string{
		"..%2F..%2Fmessages.db",
		".hidden",
		"sub%2Fpath.png",
	} {
		s := newTestServer(t, true)
		res, _ := doGET(t, s, "/api/images/predictions/"+name)
		if res.StatusCode == http.StatusOK {
			t.Errorf("name %q served with 200, want rejection", name)
		}
	}
}

func TestSafeImageName(t *testing.T) {
	ok := []string{"loaf.png", "123_roundhistory.png", "a.b.c.png"}
	bad := []string{"", ".", "..", "../x", "a/b", "a\\b", ".hidden", "..hidden", "x/../y"}
	for _, n := range ok {
		if !safeImageName(n) {
			t.Errorf("safeImageName(%q) = false, want true", n)
		}
	}
	for _, n := range bad {
		if safeImageName(n) {
			t.Errorf("safeImageName(%q) = true, want false", n)
		}
	}
}

func TestAuthRequiredWhenTokenSet(t *testing.T) {
	s := newTestServer(t, true)
	s.token = "secret" // enable auth

	// No auth header -> 401.
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no-token status = %d, want 401", rec.Code)
	}

	// Correct token -> 200.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	req.Header.Set("Authorization", "Bearer secret")
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("with-token status = %d, want 200", rec.Code)
	}

	// healthz stays open even with auth enabled.
	rec = httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz status = %d, want 200 (unauthenticated)", rec.Code)
	}
}
