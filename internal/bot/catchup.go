package bot

import (
	"log/slog"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
)

// CatchUp replays messages posted to the bread channels while the bot was
// offline. It runs once on startup, after the session is open: for each bread
// channel it fetches the most recent cfg.CatchUpLimit messages, keeps those
// newer than the stored last-read timestamp, and runs them through the normal
// plain-message pipeline (bread detection + "are you sure" retries) oldest
// first.
//
// It is a no-op when catch-up is disabled (CatchUpLimit <= 0) or when no
// last-read timestamp has been stored yet (fresh DB): without a lower bound we
// would replay entire channel histories, spamming replies. The bot records the
// timestamp on every message it sees (see recordLastRead), so the bound exists
// after the first live message post-deploy.
func (b *Bot) CatchUp() {
	if b.cfg.CatchUpLimit <= 0 {
		return
	}
	since, ok, err := b.db.GetLastReadTimestamp()
	if err != nil {
		slog.Error("catch-up: get last read timestamp", "err", err)
		return
	}
	if !ok {
		slog.Info("catch-up: no last-read timestamp stored; skipping (nothing to catch up on yet)")
		return
	}

	slog.Info("catch-up: starting", "since", since, "channels", len(b.cfg.DiscordBreadChannels), "limit", b.cfg.CatchUpLimit)
	total := 0
	for _, channelID := range b.cfg.DiscordBreadChannels {
		total += b.catchUpChannel(idString(channelID), since)
	}
	slog.Info("catch-up: done", "processed", total)
}

// catchUpChannel replays the missed messages in one channel and returns how
// many it processed.
func (b *Bot) catchUpChannel(channelID string, since time.Time) int {
	s := b.session

	// REST-fetched messages carry an empty GuildID and no Member (roles), both
	// of which the bread pipeline needs. Resolve the channel's guild once so we
	// can backfill them per message below.
	ch, err := s.Channel(channelID)
	if err != nil {
		slog.Error("catch-up: resolve channel", "channel", channelID, "err", err)
		return 0
	}

	// Newest-first, up to the limit. We don't paginate further back: the limit
	// is the deliberate ceiling on how much a single catch-up will replay.
	msgs, err := s.ChannelMessages(channelID, b.cfg.CatchUpLimit, "", "", "")
	if err != nil {
		slog.Error("catch-up: fetch messages", "channel", channelID, "err", err)
		return 0
	}

	// Process oldest-first so replies land in chronological order and the
	// last-read timestamp advances monotonically.
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].Timestamp.Before(msgs[j].Timestamp) })

	processed := 0
	for _, msg := range msgs {
		if !msg.Timestamp.After(since) {
			continue
		}
		if msg.Author == nil || msg.Author.ID == b.selfID {
			continue
		}
		mc := b.prepareCatchUpMessage(msg, ch.GuildID)
		slog.Info("catch-up: replaying message",
			"message_id", mc.ID, "channel", channelID, "author", mc.Author.Username, "timestamp", mc.Timestamp)
		b.onPlainMessage(s, mc)
		b.recordLastRead(mc.Timestamp)
		processed++
	}
	return processed
}

// prepareCatchUpMessage adapts a REST-fetched message into a *MessageCreate
// with the fields the bread pipeline relies on. It backfills the guild id and,
// for bread channels, the author's guild Member (roles) — neither of which the
// channel-messages endpoint returns.
func (b *Bot) prepareCatchUpMessage(msg *discordgo.Message, guildID string) *discordgo.MessageCreate {
	if msg.GuildID == "" {
		msg.GuildID = guildID
	}
	if msg.Member == nil && msg.GuildID != "" && msg.Author != nil {
		if member, err := b.session.GuildMember(msg.GuildID, msg.Author.ID); err == nil {
			msg.Member = member
		} else {
			slog.Warn("catch-up: fetch member (roles unknown; may skip as candidate)",
				"guild", msg.GuildID, "author_id", msg.Author.ID, "err", err)
		}
	}
	return &discordgo.MessageCreate{Message: msg}
}
