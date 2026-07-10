package bot

import (
	"database/sql"
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/hawawa4/breadbotdiscord/internal/db"
)

// onMessageCreate is the top-level message router. Mirrors on_message:
//  1. ignore the bot's own messages,
//  2. upsert the author into discordusers on every message,
//  3. dispatch $-commands, else fall through to the plain-message path.
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.ID == b.selfID {
		return
	}

	slog.Info("message received",
		"message_id", m.ID,
		"channel", m.ChannelID,
		"author", m.Author.Username,
		"author_id", m.Author.ID,
		"attachments", len(m.Attachments),
		"content_len", len(m.Content),
	)

	b.upsertAuthor(m)

	if name, args, ok := b.parseCommand(m.Content); ok {
		b.dispatchCommand(s, m, name, args)
		return
	}
	b.onPlainMessage(s, m)
}

// upsertAuthor caches the message author's name and (guild) nickname. Runs on
// every message, mirroring upsert_user_info in on_message.
func (b *Bot) upsertAuthor(m *discordgo.MessageCreate) {
	nick := ""
	if m.Member != nil {
		nick = m.Member.Nick
	}
	u := db.User{
		AuthorID:       mustParseID(m.Author.ID),
		AuthorName:     m.Author.Username,
		AuthorNickname: sql.NullString{String: nick, Valid: nick != ""},
	}
	if err := b.db.UpsertUser(u); err != nil {
		slog.Error("upsert user", "author_id", m.Author.ID, "err", err)
	}
}

// parseCommand reports whether content is a registered $-command. It returns
// the command name (without prefix) and the space-split argument list.
//
// This mirrors discord.py's dispatch: a message is a valid command only if it
// starts with the prefix AND names a registered command; anything else (plain
// text, or "$unknown") falls through to the plain-message path.
func (b *Bot) parseCommand(content string) (name string, args []string, ok bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, commandPrefix) {
		return "", nil, false
	}
	fields := strings.Fields(trimmed[len(commandPrefix):])
	if len(fields) == 0 {
		return "", nil, false
	}
	name = fields[0]
	if !isRegisteredCommand(name) {
		return "", nil, false
	}
	return name, fields[1:], true
}

// isRegisteredCommand lists the commands the bot responds to ($help, $hello,
// $breadstats). Kept in sync with dispatchCommand.
func isRegisteredCommand(name string) bool {
	switch name {
	case "help", "hello", "breadstats":
		return true
	default:
		return false
	}
}
