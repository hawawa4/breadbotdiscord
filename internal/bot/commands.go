package bot

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

// dispatchCommand routes a parsed $-command to its handler. name is the command
// without the prefix; args are the space-split arguments after it.
//
// The individual command handlers ($hello, $breadstats and its flags) are
// implemented in phase 5. This dispatcher and the parse/routing layer are in
// place now.
func (b *Bot) dispatchCommand(s *discordgo.Session, m *discordgo.MessageCreate, name string, args []string) {
	switch name {
	case "hello":
		b.cmdHello(s, m, args)
	case "breadstats":
		b.cmdBreadstats(s, m, args)
	default:
		slog.Warn("dispatch: unknown command reached dispatcher", "name", name)
	}
}

// cmdHello replies "Hello!". Mirrors the Python hello command.
func (b *Bot) cmdHello(s *discordgo.Session, m *discordgo.MessageCreate, _ []string) {
	b.reply(s, m, "Hello!")
}

// cmdBreadstats handles $breadstats and its flags. Fleshed out in phase 5.
func (b *Bot) cmdBreadstats(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	slog.Debug("breadstats (not yet implemented)", "args", args)
}

// reply sends content as a reply to the triggering message, matching the
// Python `reference=ctx.message` behavior.
func (b *Bot) reply(s *discordgo.Session, m *discordgo.MessageCreate, content string) {
	if _, err := s.ChannelMessageSendReply(m.ChannelID, content, m.Reference()); err != nil {
		slog.Error("send reply", "channel", m.ChannelID, "err", err)
	}
}
