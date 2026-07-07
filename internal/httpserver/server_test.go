package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	return New(":0", database, fakeBot{ready: botReady}, "")
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
		AuthorID int64            `json:"author_id"`
		Min      *messageDTO      `json:"min"`
		Max      *messageDTO      `json:"max"`
		History  []map[string]any `json:"history"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AuthorID != knownUser {
		t.Errorf("author_id = %d, want %d", out.AuthorID, knownUser)
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
