package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// lastReadKey is the botstate key under which the "catch up" feature stores the
// timestamp of the most recent message the bot has processed.
const lastReadKey = "last_read_timestamp"

// GetLastReadTimestamp returns the timestamp of the most recent message the bot
// has processed, and whether one was stored. The second return is false on a
// fresh DB (nothing recorded yet) so callers can skip catch-up entirely rather
// than replaying the whole channel history.
func (d *DB) GetLastReadTimestamp() (time.Time, bool, error) {
	const q = "SELECT value FROM botstate WHERE key = ?"
	var value string
	err := d.sql.QueryRow(q, lastReadKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("db: get last read timestamp: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("db: parse last read timestamp %q: %w", value, err)
	}
	return t, true, nil
}

// SetLastReadTimestamp records t as the most recent message the bot has
// processed. It is stored as RFC3339Nano text so ordering is preserved.
func (d *DB) SetLastReadTimestamp(t time.Time) error {
	const q = `
	INSERT INTO botstate (key, value)
	VALUES (?, ?)
	ON CONFLICT(key) DO UPDATE SET value=excluded.value`
	if _, err := d.sql.Exec(q, lastReadKey, t.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("db: set last read timestamp: %w", err)
	}
	return nil
}
