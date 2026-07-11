// Package db is the SQLite access layer, shared by the bot and the HTTP server.
//
// It reuses the existing dbdata/messages.db unchanged (same schema as the
// Python implementation in src/db/). Uses the pure-Go modernc.org/sqlite
// driver so builds need no CGO.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// ErrUserNotFound is returned when a user/roundness lookup yields no rows.
// It mirrors the Python UserNotFound exception.
var ErrUserNotFound = fmt.Errorf("user not found")

// OrderBy is the sort direction for roundness queries.
type OrderBy string

const (
	OrderAsc  OrderBy = "ASC"
	OrderDesc OrderBy = "DESC"
)

// DB wraps a *sql.DB connection to the messages database.
type DB struct {
	sql *sql.DB
}

// Open opens (or creates) the SQLite database at path, ensures its parent
// directory exists, and creates the schema if missing.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("db: create dir %q: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("db: open %q: %w", path, err)
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("db: ping %q: %w", path, err)
	}

	d := &DB{sql: sqlDB}
	if err := d.createSchema(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

// Ping reports whether the database is reachable (used by /healthz).
func (d *DB) Ping() error { return d.sql.Ping() }

// Close closes the underlying connection pool.
func (d *DB) Close() error { return d.sql.Close() }

// createSchema creates the messages and discordusers tables if they don't
// already exist. Identical to src/db/service.py create_db().
func (d *DB) createSchema() error {
	const createMessages = `
	CREATE TABLE IF NOT EXISTS messages (
		ogmessage_id INTEGER PRIMARY KEY,
		replymessage_jump_url TEXT,
		replymessage_id INTEGER,
		author_id INTEGER,
		channel_id INTEGER,
		guild_id INTEGER,
		roundness REAL,
		labels_json TEXT
	)`
	const createUsers = `
	CREATE TABLE IF NOT EXISTS discordusers (
		author_id INTEGER PRIMARY KEY,
		author_nickname TEXT,
		author_name TEXT
	)`
	// botstate is a small key/value table for runtime bookkeeping. It has no
	// Python counterpart; it backs the "catch up" feature (see GetBotState /
	// SetBotState), which stores a single last-read timestamp rather than a row
	// per message.
	const createBotState = `
	CREATE TABLE IF NOT EXISTS botstate (
		key TEXT PRIMARY KEY,
		value TEXT
	)`

	if _, err := d.sql.Exec(createMessages); err != nil {
		return fmt.Errorf("db: create messages table: %w", err)
	}
	if _, err := d.sql.Exec(createUsers); err != nil {
		return fmt.Errorf("db: create discordusers table: %w", err)
	}
	if _, err := d.sql.Exec(createBotState); err != nil {
		return fmt.Errorf("db: create botstate table: %w", err)
	}
	return nil
}
