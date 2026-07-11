package db

import (
	"database/sql"
	"encoding/json"
)

// Message mirrors a row in the `messages` table.
//
// A row is one image attachment of a Discord message: the primary key is the
// composite (OgMessageID, AttachmentID). A message with several images has
// several rows, each scored independently. Legacy rows migrated from the
// single-key schema have AttachmentID 0.
//
// Roundness and Labels are nullable in the schema (a message can have discord
// info persisted before inference results, or vice versa). Roundness uses
// sql.NullFloat64; Labels is nil when labels_json is NULL/empty.
type Message struct {
	OgMessageID         int64
	AttachmentID        int64
	ReplyMessageJumpURL string
	ReplyMessageID      int64
	AuthorID            int64
	ChannelID           int64
	GuildID             int64
	Roundness           sql.NullFloat64
	Labels              map[string]float64
	// ImageFilename is the basename of the annotated prediction PNG saved under
	// downloads/predictions/, or invalid if none was produced/persisted.
	ImageFilename sql.NullString
}

// messageColumns is the fixed column order used by every SELECT and by scanRow.
const messageColumns = "ogmessage_id,attachment_id,replymessage_jump_url,replymessage_id,author_id,channel_id,guild_id,roundness,labels_json,image_filename"

// selectMessages is the base SELECT for the messages table.
const selectMessages = "SELECT " + messageColumns + " FROM messages"

// scanMessage scans one row (in messageColumns order) into a Message.
// Nullable text/int columns are tolerated via sql.Null* scanning.
func scanMessage(s interface{ Scan(...any) error }) (Message, error) {
	var (
		m          Message
		jumpURL    sql.NullString
		replyID    sql.NullInt64
		authorID   sql.NullInt64
		channelID  sql.NullInt64
		guildID    sql.NullInt64
		labelsJSON sql.NullString
	)
	if err := s.Scan(
		&m.OgMessageID,
		&m.AttachmentID,
		&jumpURL,
		&replyID,
		&authorID,
		&channelID,
		&guildID,
		&m.Roundness,
		&labelsJSON,
		&m.ImageFilename,
	); err != nil {
		return Message{}, err
	}
	m.ReplyMessageJumpURL = jumpURL.String
	m.ReplyMessageID = replyID.Int64
	m.AuthorID = authorID.Int64
	m.ChannelID = channelID.Int64
	m.GuildID = guildID.Int64
	if labelsJSON.Valid && labelsJSON.String != "" {
		if err := json.Unmarshal([]byte(labelsJSON.String), &m.Labels); err != nil {
			return Message{}, err
		}
	}
	return m, nil
}

// User mirrors a row in the `discordusers` table.
type User struct {
	AuthorID       int64
	AuthorNickname sql.NullString
	AuthorName     string
}

// nullString wraps a non-empty string as a valid sql.NullString; an empty
// string becomes NULL.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
