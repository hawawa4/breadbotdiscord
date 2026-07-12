package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// UpsertUser inserts or updates a discord user's cached name/nickname, keyed on
// author_id. Called on every message. Mirrors upsert_user_info.
func (d *DB) UpsertUser(u User) error {
	const q = `
	INSERT INTO discordusers (author_id, author_nickname, author_name)
	VALUES (?, ?, ?)
	ON CONFLICT(author_id) DO UPDATE SET
		author_nickname=excluded.author_nickname,
		author_name=excluded.author_name`
	if _, err := d.sql.Exec(q, u.AuthorID, u.AuthorNickname, u.AuthorName); err != nil {
		return fmt.Errorf("db: upsert user: %w", err)
	}
	return nil
}

// ListUsers returns cached users ordered by author_id, limited to limit rows
// starting at offset (for a paginated user directory in the HTTP API). limit is
// clamped to at least 1 by the caller; offset < 0 is treated as 0.
func (d *DB) ListUsers(limit, offset int) ([]User, error) {
	if offset < 0 {
		offset = 0
	}
	const q = `
	SELECT author_id, author_nickname, author_name
	FROM discordusers
	ORDER BY author_id
	LIMIT ? OFFSET ?`
	rows, err := d.sql.Query(q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("db: list users: %w", err)
	}
	defer rows.Close()

	var result []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.AuthorID, &u.AuthorNickname, &u.AuthorName); err != nil {
			return nil, fmt.Errorf("db: scan user row: %w", err)
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

// CountUsers returns the total number of cached users (for pagination totals).
func (d *DB) CountUsers() (int, error) {
	var n int
	if err := d.sql.QueryRow("SELECT COUNT(*) FROM discordusers").Scan(&n); err != nil {
		return 0, fmt.Errorf("db: count users: %w", err)
	}
	return n, nil
}

// SelectUsersByIDs returns cached info for the given author IDs, keyed by
// author_id. IDs with no cached discordusers row are simply absent from the map
// (the caller falls back to the bare id). Duplicate ids are de-duplicated. A
// single query avoids N+1 lookups when enriching a list of messages.
func (d *DB) SelectUsersByIDs(ids []int64) (map[int64]User, error) {
	out := make(map[int64]User)
	if len(ids) == 0 {
		return out, nil
	}
	// De-duplicate while building the placeholder list and args.
	seen := make(map[int64]struct{}, len(ids))
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	q := "SELECT author_id, author_nickname, author_name FROM discordusers WHERE author_id IN (" +
		strings.Join(placeholders, ",") + ")"
	rows, err := d.sql.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("db: select users by ids: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var u User
		if err := rows.Scan(&u.AuthorID, &u.AuthorNickname, &u.AuthorName); err != nil {
			return nil, fmt.Errorf("db: scan user row: %w", err)
		}
		out[u.AuthorID] = u
	}
	return out, rows.Err()
}

// SelectUser returns cached info for a user, or ErrUserNotFound if absent.
// Mirrors select_user_info.
func (d *DB) SelectUser(authorID int64) (User, error) {
	const q = "SELECT author_id, author_nickname, author_name FROM discordusers WHERE author_id = ?"
	var u User
	err := d.sql.QueryRow(q, authorID).Scan(&u.AuthorID, &u.AuthorNickname, &u.AuthorName)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("db: select user: %w", err)
	}
	return u, nil
}
