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
// already exist, then applies backward-compatible migrations for DB files
// created by earlier versions.
//
// The messages table is keyed by the composite (ogmessage_id, attachment_id):
// a single Discord message can carry several image attachments, each of which
// is inferred and scored independently, so one row per (message, attachment) is
// required. Older DBs keyed by ogmessage_id alone are rebuilt in place with the
// pre-existing rows assigned attachment_id 0 (see migrateMessagesSchema).
func (d *DB) createSchema() error {
	const createMessages = `
	CREATE TABLE IF NOT EXISTS messages (
		ogmessage_id INTEGER NOT NULL,
		attachment_id INTEGER NOT NULL DEFAULT 0,
		replymessage_jump_url TEXT,
		replymessage_id INTEGER,
		author_id INTEGER,
		channel_id INTEGER,
		guild_id INTEGER,
		roundness REAL,
		labels_json TEXT,
		image_filename TEXT,
		PRIMARY KEY (ogmessage_id, attachment_id)
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

	if err := d.migrateMessagesSchema(); err != nil {
		return err
	}
	return nil
}

// migrateMessagesSchema brings a pre-existing messages table up to the current
// shape. It is idempotent (safe to run on every open) and handles two eras of
// DB file:
//
//   - Pre-frontend: no image_filename column. Added idempotently (nullable, so
//     old rows simply have no linked gallery image).
//   - Pre-multi-image: keyed by ogmessage_id alone, no attachment_id. The table
//     is rebuilt with the composite PK and existing rows get attachment_id 0.
//
// A freshly created table (from createSchema's CREATE) already has both, so
// both checks no-op.
func (d *DB) migrateMessagesSchema() error {
	cols, err := d.tableColumns("messages")
	if err != nil {
		return err
	}

	if !cols["image_filename"] {
		if err := d.ensureColumn("messages", "image_filename", "TEXT"); err != nil {
			return err
		}
	}

	if !cols["attachment_id"] {
		if err := d.rebuildMessagesWithAttachmentID(); err != nil {
			return err
		}
	}
	return nil
}

// rebuildMessagesWithAttachmentID rebuilds a legacy messages table (PK on
// ogmessage_id only) into the composite-PK shape. SQLite cannot alter a primary
// key in place, so this creates the new table, copies every row with
// attachment_id 0, drops the old table, and renames — all inside a transaction.
func (d *DB) rebuildMessagesWithAttachmentID() error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("db: begin messages rebuild: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only if Commit didn't run

	stmts := []string{
		`CREATE TABLE messages_new (
			ogmessage_id INTEGER NOT NULL,
			attachment_id INTEGER NOT NULL DEFAULT 0,
			replymessage_jump_url TEXT,
			replymessage_id INTEGER,
			author_id INTEGER,
			channel_id INTEGER,
			guild_id INTEGER,
			roundness REAL,
			labels_json TEXT,
			image_filename TEXT,
			PRIMARY KEY (ogmessage_id, attachment_id)
		)`,
		`INSERT INTO messages_new
			(ogmessage_id, attachment_id, replymessage_jump_url, replymessage_id,
			 author_id, channel_id, guild_id, roundness, labels_json, image_filename)
		 SELECT
			ogmessage_id, 0, replymessage_jump_url, replymessage_id,
			author_id, channel_id, guild_id, roundness, labels_json, image_filename
		 FROM messages`,
		`DROP TABLE messages`,
		`ALTER TABLE messages_new RENAME TO messages`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("db: rebuild messages: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit messages rebuild: %w", err)
	}
	return nil
}

// tableColumns returns the set of column names present on a table.
func (d *DB) tableColumns(table string) (map[string]bool, error) {
	rows, err := d.sql.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("db: inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid        int
			name, typ  string
			notNull    int
			dflt       any
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &primaryKey); err != nil {
			return nil, fmt.Errorf("db: scan %s column: %w", table, err)
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// ensureColumn adds a column to a table if it is not already present. Used for
// additive, backward-compatible migrations against a pre-existing DB file
// (SQLite has no "ADD COLUMN IF NOT EXISTS", so we check PRAGMA table_info).
func (d *DB) ensureColumn(table, column, decl string) error {
	cols, err := d.tableColumns(table)
	if err != nil {
		return err
	}
	if cols[column] {
		return nil
	}
	if _, err := d.sql.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl)); err != nil {
		return fmt.Errorf("db: add column %s.%s: %w", table, column, err)
	}
	return nil
}
