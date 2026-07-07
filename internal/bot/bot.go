// Package bot is the discordgo-based Discord client: session setup, intents,
// event routing, and the bread-detection + stats command handlers.
//
// It ports src/discordclient/. discordgo has no built-in command framework
// (unlike discord.py's commands.Bot), so command dispatch is done explicitly in
// messages.go.
package bot

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"

	"github.com/hawawa4/breadbotdiscord/internal/config"
	"github.com/hawawa4/breadbotdiscord/internal/db"
	"github.com/hawawa4/breadbotdiscord/internal/inference"
)

// commandPrefix matches the Python command_prefix="$".
const commandPrefix = "$"

// Bot wires the discord session to the DB and inference client.
type Bot struct {
	cfg       *config.Config
	db        *db.DB
	inference *inference.Client
	session   *discordgo.Session

	// selfID is the bot's own user id, set on ready; used to ignore its own
	// messages and to detect replies to the bot.
	selfID string
}

// New constructs a Bot with an authenticated (but not yet open) session and the
// message-content intent enabled.
func New(cfg *config.Config, database *db.DB, inf *inference.Client) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, err
	}
	// discord.Intents.default() + message_content.
	session.Identify.Intents = discordgo.IntentsAllWithoutPrivileged | discordgo.IntentMessageContent

	b := &Bot{
		cfg:       cfg,
		db:        database,
		inference: inf,
		session:   session,
	}

	session.AddHandler(b.onReady)
	session.AddHandler(b.onMessageCreate)
	return b, nil
}

// Open connects the session to Discord (starts the websocket).
func (b *Bot) Open() error { return b.session.Open() }

// Close tears down the session.
func (b *Bot) Close() error { return b.session.Close() }

// Ready reports whether the session has an established gateway connection
// (used by /healthz).
func (b *Bot) Ready() bool {
	return b.selfID != ""
}

// onReady logs in and records the bot's own user id. Mirrors on_ready.
func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	b.selfID = r.User.ID
	slog.Info("logged in", "user", r.User.String())
}
