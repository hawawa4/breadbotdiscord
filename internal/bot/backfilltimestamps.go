package bot

import (
	"log/slog"
)

// BackfillTimestamps fills in the created_at column for messages persisted
// before the timestamp was stored (see UpsertMessageDiscordInfo). It runs once
// on startup, after the session is open, in the same spirit as CatchUp: for
// each message missing a timestamp it re-fetches the original message over REST
// and stores its creation time in unix milliseconds.
//
// It is best-effort and logged: a message that can't be fetched (deleted, or
// the bot lost access) is simply skipped and retried on a future startup. New
// messages never need this — the live pipeline stores the timestamp inline.
func (b *Bot) BackfillTimestamps() {
	missing, err := b.db.MessagesMissingCreatedAt()
	if err != nil {
		slog.Error("backfill timestamps: query missing", "err", err)
		return
	}
	if len(missing) == 0 {
		return
	}

	slog.Info("backfill timestamps: starting", "messages", len(missing))
	filled, skipped := 0, 0
	for _, m := range missing {
		channelID := idString(m.ChannelID)
		messageID := idString(m.OgMessageID)
		msg, err := b.session.ChannelMessage(channelID, messageID)
		if err != nil {
			// Deleted or inaccessible: leave it null and retry next startup.
			slog.Warn("backfill timestamps: fetch message",
				"message_id", messageID, "channel", channelID, "err", err)
			skipped++
			continue
		}
		if msg.Timestamp.IsZero() {
			skipped++
			continue
		}
		if err := b.db.SetMessageCreatedAt(m.OgMessageID, msg.Timestamp.UnixMilli()); err != nil {
			slog.Error("backfill timestamps: store", "message_id", messageID, "err", err)
			skipped++
			continue
		}
		filled++
	}
	slog.Info("backfill timestamps: done", "filled", filled, "skipped", skipped)
}
