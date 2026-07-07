package db

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// openTestDB copies the committed dbdata/messages.db into a temp dir and opens
// it, so tests never mutate the real fixture.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	src := filepath.Join("..", "..", "dbdata", "messages.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	defer in.Close()

	dst := filepath.Join(t.TempDir(), "messages.db")
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatalf("copy db: %v", err)
	}
	out.Close()

	d, err := Open(dst)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

const (
	userWithHistory  = int64(95618667529637888)
	roundestOgID     = int64(1419411009986367640)
	leastRoundOgID   = int64(1266697604218224650)
	totalRoundnessN  = 32
)

func TestOpenCreatesSchemaOnFreshDB(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "nested", "fresh.db")
	d, err := Open(dst)
	if err != nil {
		t.Fatalf("Open fresh: %v", err)
	}
	defer d.Close()
	// Upsert then read back to confirm tables exist.
	if err := d.UpsertUser(User{AuthorID: 1, AuthorName: "x"}); err != nil {
		t.Fatalf("UpsertUser on fresh db: %v", err)
	}
	if _, err := d.SelectUser(1); err != nil {
		t.Fatalf("SelectUser on fresh db: %v", err)
	}
}

func TestMaxRoundnessLeaderboard(t *testing.T) {
	d := openTestDB(t)
	got, err := d.GetMaxRoundnessLeaderboard(3)
	if err != nil {
		t.Fatalf("GetMaxRoundnessLeaderboard: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Rows must be sorted by roundness descending.
	for i := 1; i < len(got); i++ {
		if got[i-1].Roundness.Float64 < got[i].Roundness.Float64 {
			t.Errorf("not descending: %v < %v", got[i-1].Roundness.Float64, got[i].Roundness.Float64)
		}
	}
}

func TestLeaderboardLimitExceedsRows(t *testing.T) {
	d := openTestDB(t)
	got, err := d.GetMinRoundnessLeaderboard(1000)
	if err != nil {
		t.Fatalf("GetMinRoundnessLeaderboard: %v", err)
	}
	if len(got) != totalRoundnessN {
		t.Errorf("len = %d, want %d (all roundness rows)", len(got), totalRoundnessN)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Roundness.Float64 > got[i].Roundness.Float64 {
			t.Errorf("not ascending: %v > %v", got[i-1].Roundness.Float64, got[i].Roundness.Float64)
		}
	}
}

func TestRoundnessForUser(t *testing.T) {
	d := openTestDB(t)

	maxMsg, err := d.GetMaxRoundnessForUser(userWithHistory)
	if err != nil {
		t.Fatalf("GetMaxRoundnessForUser: %v", err)
	}
	if maxMsg.OgMessageID != 1271744436119801937 {
		t.Errorf("max og id = %d, want 1271744436119801937", maxMsg.OgMessageID)
	}
	if !maxMsg.Roundness.Valid {
		t.Error("max roundness should be valid")
	}

	minMsg, err := d.GetMinRoundnessForUser(userWithHistory)
	if err != nil {
		t.Fatalf("GetMinRoundnessForUser: %v", err)
	}
	if minMsg.OgMessageID != 1266693487877685249 {
		t.Errorf("min og id = %d, want 1266693487877685249", minMsg.OgMessageID)
	}
	if minMsg.Roundness.Float64 > maxMsg.Roundness.Float64 {
		t.Errorf("min %v should be <= max %v", minMsg.Roundness.Float64, maxMsg.Roundness.Float64)
	}
}

func TestRoundnessForUserNotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetMaxRoundnessForUser(999999999)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestRoundnessHistory(t *testing.T) {
	d := openTestDB(t)
	hist, err := d.GetRoundnessHistory(userWithHistory)
	if err != nil {
		t.Fatalf("GetRoundnessHistory: %v", err)
	}
	if len(hist) != 10 {
		t.Fatalf("history len = %d, want 10", len(hist))
	}
	// Index is 1-based and monotonically increasing.
	for i, p := range hist {
		if p.Index != i+1 {
			t.Errorf("hist[%d].Index = %d, want %d", i, p.Index, i+1)
		}
	}
}

func TestGetMessageLabels(t *testing.T) {
	d := openTestDB(t)
	m, err := d.GetMessage(roundestOgID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if !m.Roundness.Valid {
		t.Error("roundness should be valid")
	}
	if v, ok := m.Labels["bread"]; !ok || v <= 0 {
		t.Errorf("expected a positive 'bread' label, got %v (ok=%v)", v, ok)
	}
}

func TestUpsertMessageStatsRoundTrip(t *testing.T) {
	d := openTestDB(t)
	labels := map[string]float64{"bread": 0.9, "round": 0.5}
	if err := d.UpsertMessageStats(roundestOgID, 0.77, labels); err != nil {
		t.Fatalf("UpsertMessageStats: %v", err)
	}
	m, err := d.GetMessage(roundestOgID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if m.Roundness.Float64 != 0.77 {
		t.Errorf("roundness = %v, want 0.77", m.Roundness.Float64)
	}
	if m.Labels["bread"] != 0.9 {
		t.Errorf("labels[bread] = %v, want 0.9", m.Labels["bread"])
	}
}

func TestUserRoundTrip(t *testing.T) {
	d := openTestDB(t)
	u := User{AuthorID: 42, AuthorName: "loaf", AuthorNickname: nullString("Loafy")}
	if err := d.UpsertUser(u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	got, err := d.SelectUser(42)
	if err != nil {
		t.Fatalf("SelectUser: %v", err)
	}
	if got.AuthorName != "loaf" || got.AuthorNickname.String != "Loafy" {
		t.Errorf("got %+v, want name=loaf nick=Loafy", got)
	}
}
