package db

import (
	"testing"
	"time"
)

func TestLastReadTimestampAbsentOnFreshDB(t *testing.T) {
	d := openTestDB(t)
	_, ok, err := d.GetLastReadTimestamp()
	if err != nil {
		t.Fatalf("GetLastReadTimestamp: %v", err)
	}
	if ok {
		t.Fatal("expected no last-read timestamp on a DB that never stored one")
	}
}

func TestLastReadTimestampRoundTrip(t *testing.T) {
	d := openTestDB(t)
	// Include sub-second precision to confirm RFC3339Nano storage preserves it.
	want := time.Date(2026, 7, 11, 12, 30, 45, 123456789, time.UTC)
	if err := d.SetLastReadTimestamp(want); err != nil {
		t.Fatalf("SetLastReadTimestamp: %v", err)
	}
	got, ok, err := d.GetLastReadTimestamp()
	if err != nil {
		t.Fatalf("GetLastReadTimestamp: %v", err)
	}
	if !ok {
		t.Fatal("expected a stored timestamp")
	}
	if !got.Equal(want) {
		t.Fatalf("round-trip mismatch: got %v, want %v", got, want)
	}
}

func TestSetLastReadTimestampOverwrites(t *testing.T) {
	d := openTestDB(t)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	if err := d.SetLastReadTimestamp(first); err != nil {
		t.Fatalf("SetLastReadTimestamp first: %v", err)
	}
	if err := d.SetLastReadTimestamp(second); err != nil {
		t.Fatalf("SetLastReadTimestamp second: %v", err)
	}
	got, _, err := d.GetLastReadTimestamp()
	if err != nil {
		t.Fatalf("GetLastReadTimestamp: %v", err)
	}
	if !got.Equal(second) {
		t.Fatalf("expected overwrite to %v, got %v", second, got)
	}
}

// Timezone is normalized to UTC on store; a non-UTC input must read back as the
// same instant.
func TestLastReadTimestampNormalizesTimezone(t *testing.T) {
	d := openTestDB(t)
	loc := time.FixedZone("test+5", 5*60*60)
	in := time.Date(2026, 7, 11, 17, 0, 0, 0, loc)
	if err := d.SetLastReadTimestamp(in); err != nil {
		t.Fatalf("SetLastReadTimestamp: %v", err)
	}
	got, _, err := d.GetLastReadTimestamp()
	if err != nil {
		t.Fatalf("GetLastReadTimestamp: %v", err)
	}
	if !got.Equal(in) {
		t.Fatalf("expected same instant %v, got %v", in.UTC(), got)
	}
}
