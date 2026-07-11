package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// UpsertMessageStats inserts or updates the inference results (roundness +
// labels + annotated-image filename) for a message, keyed on ogmessage_id.
// labels is serialized to a JSON string. imageFilename is the basename of the
// annotated PNG under downloads/predictions/, or "" when none was produced (in
// which case the column is left NULL). Mirrors upsert_message_stats.
func (d *DB) UpsertMessageStats(ogMessageID int64, roundness float64, labels map[string]float64, imageFilename string) error {
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("db: marshal labels: %w", err)
	}
	const q = `
	INSERT INTO messages (ogmessage_id, roundness, labels_json, image_filename)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(ogmessage_id) DO UPDATE SET
		roundness=excluded.roundness,
		labels_json=excluded.labels_json,
		image_filename=excluded.image_filename`
	if _, err := d.sql.Exec(q, ogMessageID, roundness, string(labelsJSON), nullString(imageFilename)); err != nil {
		return fmt.Errorf("db: upsert message stats: %w", err)
	}
	return nil
}

// UpsertMessageDiscordInfo inserts or updates the discord metadata for a
// message, keyed on ogmessage_id. Mirrors upsert_message_discordinfo.
func (d *DB) UpsertMessageDiscordInfo(ogMessageID int64, replyJumpURL string, replyMessageID, authorID, channelID, guildID int64) error {
	const q = `
	INSERT INTO messages (ogmessage_id, replymessage_jump_url, replymessage_id, author_id, channel_id, guild_id)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(ogmessage_id) DO UPDATE SET
		replymessage_jump_url=excluded.replymessage_jump_url,
		replymessage_id=excluded.replymessage_id,
		author_id=excluded.author_id,
		channel_id=excluded.channel_id,
		guild_id=excluded.guild_id`
	if _, err := d.sql.Exec(q, ogMessageID, replyJumpURL, replyMessageID, authorID, channelID, guildID); err != nil {
		return fmt.Errorf("db: upsert message discord info: %w", err)
	}
	return nil
}

// GetMessage returns a single message by ogmessage_id (for the HTTP server).
func (d *DB) GetMessage(ogMessageID int64) (Message, error) {
	row := d.sql.QueryRow(selectMessages+" WHERE ogmessage_id = ?", ogMessageID)
	m, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrUserNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("db: get message: %w", err)
	}
	return m, nil
}

// GetMinRoundnessForUser returns the least-round message for a user.
func (d *DB) GetMinRoundnessForUser(userID int64) (Message, error) {
	return d.roundnessMessageByUser(userID, OrderAsc)
}

// GetMaxRoundnessForUser returns the roundest message for a user.
func (d *DB) GetMaxRoundnessForUser(userID int64) (Message, error) {
	return d.roundnessMessageByUser(userID, OrderDesc)
}

// roundnessMessageByUser returns the single min/max roundness message for a
// user. Mirrors _get_roundness_message_byuserid: ties broken by ogmessage_id
// in the same direction as roundness. Returns ErrUserNotFound if none.
func (d *DB) roundnessMessageByUser(userID int64, order OrderBy) (Message, error) {
	// A roundness of 0 means the shape couldn't be computed (effectively null),
	// so exclude it from min/max ranking just like a NULL.
	q := fmt.Sprintf(`
	%s
	WHERE author_id = ?
	AND roundness NOT NULL
	AND roundness != 0
	ORDER BY roundness %s, ogmessage_id %s
	LIMIT 1`, selectMessages, order, order)
	row := d.sql.QueryRow(q, userID)
	m, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrUserNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("db: roundness for user: %w", err)
	}
	return m, nil
}

// GetMaxRoundnessLeaderboard returns the n roundest messages server-wide.
func (d *DB) GetMaxRoundnessLeaderboard(n int) ([]Message, error) {
	return d.roundnessLeaderboard(n, OrderDesc)
}

// GetMinRoundnessLeaderboard returns the n least-round messages server-wide.
func (d *DB) GetMinRoundnessLeaderboard(n int) ([]Message, error) {
	return d.roundnessLeaderboard(n, OrderAsc)
}

// roundnessLeaderboard returns up to n min/max roundness messages server-wide.
// Mirrors _get_minmax_roundness_leaderboard.
func (d *DB) roundnessLeaderboard(n int, order OrderBy) ([]Message, error) {
	// Exclude roundness 0 (shape couldn't be computed) so it never shows up as
	// a "worst" result — it is effectively a null, not a real low score.
	q := fmt.Sprintf(`
	%s
	WHERE roundness not null
	AND roundness != 0
	ORDER BY roundness %s
	LIMIT ?`, selectMessages, order)
	rows, err := d.sql.Query(q, n)
	if err != nil {
		return nil, fmt.Errorf("db: roundness leaderboard: %w", err)
	}
	defer rows.Close()

	var result []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("db: scan leaderboard row: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// StatsSummary holds server-wide aggregate stats for the dashboard header.
// AvgRoundness/MaxRoundness are only meaningful when ScoredCount > 0.
type StatsSummary struct {
	ScoredCount  int     // messages with a real (non-null, non-zero) roundness
	DistinctUsers int    // distinct authors among scored messages
	AvgRoundness float64 // mean roundness over scored messages
	MaxRoundness float64 // highest roundness over scored messages
}

// GetStatsSummary returns server-wide aggregates over messages that have a real
// roundness score. It applies the same "roundness NOT NULL AND != 0" filter as
// the leaderboard/history queries (a 0 means the shape couldn't be computed).
func (d *DB) GetStatsSummary() (StatsSummary, error) {
	const q = `
	SELECT
		COUNT(*),
		COUNT(DISTINCT author_id),
		COALESCE(AVG(roundness), 0),
		COALESCE(MAX(roundness), 0)
	FROM messages
	WHERE roundness NOT NULL AND roundness != 0`
	var s StatsSummary
	err := d.sql.QueryRow(q).Scan(&s.ScoredCount, &s.DistinctUsers, &s.AvgRoundness, &s.MaxRoundness)
	if err != nil {
		return StatsSummary{}, fmt.Errorf("db: stats summary: %w", err)
	}
	return s, nil
}

// RoundnessPoint is one point in a user's roundness history: a 1-based index
// (most recent first) and the roundness value.
type RoundnessPoint struct {
	Index     int
	Roundness float64
}

// GetRoundnessHistory returns the last 50 roundness values for a user, ordered
// by message id descending, indexed from 1. Mirrors get_roundness_history.
func (d *DB) GetRoundnessHistory(userID int64) ([]RoundnessPoint, error) {
	// Exclude roundness 0 (shape couldn't be computed) so the plot only shows
	// real scores — a 0 is effectively a null.
	q := fmt.Sprintf(`
	%s
	WHERE 1=1
	AND roundness not null
	AND roundness != 0
	AND author_id = ?
	ORDER BY ogmessage_id %s
	LIMIT 50`, selectMessages, OrderDesc)
	rows, err := d.sql.Query(q, userID)
	if err != nil {
		return nil, fmt.Errorf("db: roundness history: %w", err)
	}
	defer rows.Close()

	var result []RoundnessPoint
	i := 1
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("db: scan history row: %w", err)
		}
		result = append(result, RoundnessPoint{Index: i, Roundness: m.Roundness.Float64})
		i++
	}
	return result, rows.Err()
}
