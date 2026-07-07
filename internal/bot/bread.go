package bot

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

// onPlainMessage handles non-command messages: the bread-detection pipeline
// (candidate detection, "are you sure" retry, inference, persistence).
//
// Implemented in phase 4. For now it is a no-op so routing can be wired and
// tested independently.
func (b *Bot) onPlainMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	slog.Debug("plain message (pipeline not yet implemented)", "message_id", m.ID)
}
