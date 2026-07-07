package db

import (
	"database/sql"
	"errors"
	"fmt"
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
