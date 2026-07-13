package bot

// TEMPORARY HOTFIX — DELETE ME.
//
// This whole file is a one-shot data-recovery job for messages whose stats rows
// were lost. It re-runs inference over a window of already-posted bread messages
// and writes the DB rows *without* sending any Discord reply (unlike the normal
// pipeline / CatchUp). It reuses the bot's inference client, download helpers,
// and persistence, but is deliberately isolated in one file gated by one env
// var so it can be removed in a single delete once the recovery deploy is done.
//
// Removal checklist:
//   - delete this file
//   - delete the `go discordBot.ReplayHotfix()` line in cmd/breadbot/main.go
//
// Trigger + bounds are read straight from the environment here (NOT wired into
// internal/config) so nothing outside this file has to change:
//
//	REPLAY_HOTFIX=1            enable the job (any other value / unset = no-op)
//	REPLAY_HOTFIX_AFTER_ID     only replay messages with id strictly greater (older bound, exclusive)
//	REPLAY_HOTFIX_BEFORE_ID    only replay messages with id strictly less    (newer bound, exclusive)
//
// Discord snowflake ids are time-ordered, so the id bounds are a time window.
// Either bound may be omitted (open-ended on that side), but supplying both is
// strongly recommended so the job stays surgical.

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/hawawa4/breadbotdiscord/internal/inference"
)

// ReplayHotfix runs the temporary recovery job when REPLAY_HOTFIX=1. It walks
// each configured bread channel between the id bounds, re-infers every bread
// candidate, and persists the stats — silently, with no reply sent.
func (b *Bot) ReplayHotfix() {
	if os.Getenv("REPLAY_HOTFIX") != "1" {
		return
	}

	afterID := parseOptionalID("REPLAY_HOTFIX_AFTER_ID")
	beforeID := parseOptionalID("REPLAY_HOTFIX_BEFORE_ID")
	slog.Warn("REPLAY HOTFIX: starting silent replay (no replies will be sent)",
		"after_id", afterID, "before_id", beforeID, "channels", len(b.cfg.DiscordBreadChannels))

	total := 0
	for _, channelID := range b.cfg.DiscordBreadChannels {
		total += b.replayChannel(idString(channelID), afterID, beforeID)
	}
	slog.Warn("REPLAY HOTFIX: done", "attachments_persisted", total)
}

// replayChannel paginates one channel between the bounds (oldest-first via the
// "after" cursor), replays each bread candidate, and returns how many
// attachments it persisted.
func (b *Bot) replayChannel(channelID string, afterID, beforeID int64) int {
	s := b.session

	// REST-fetched messages carry an empty GuildID and no Member (roles), both of
	// which the candidate check needs. Resolve the channel's guild once.
	ch, err := s.Channel(channelID)
	if err != nil {
		slog.Error("REPLAY HOTFIX: resolve channel", "channel", channelID, "err", err)
		return 0
	}

	// Page forward from the lower bound using the "after" cursor. Discord returns
	// newest-first per page, so we sort each page oldest-first and advance the
	// cursor to the newest id we saw. cursor "" starts at the channel beginning
	// (Discord treats after="" as after=0), which respects afterID via the filter
	// below; if afterID is set we can seed the cursor with it directly.
	cursor := ""
	if afterID > 0 {
		cursor = strconv.FormatInt(afterID, 10)
	}

	persisted := 0
	for {
		msgs, err := s.ChannelMessages(channelID, 100, "", cursor, "")
		if err != nil {
			slog.Error("REPLAY HOTFIX: fetch messages", "channel", channelID, "err", err)
			return persisted
		}
		if len(msgs) == 0 {
			break
		}
		sort.Slice(msgs, func(i, j int) bool { return msgs[i].Timestamp.Before(msgs[j].Timestamp) })

		reachedTop := false
		for _, msg := range msgs {
			id := mustParseID(msg.ID)
			if beforeID > 0 && id >= beforeID {
				// Snowflakes only grow; once we pass the upper bound we're done.
				reachedTop = true
				break
			}
			mc := b.prepareCatchUpMessage(msg, ch.GuildID)
			if !b.isBreadCandidate(mc) {
				continue
			}
			persisted += b.replayMessage(msg)
		}
		if reachedTop {
			break
		}
		// Advance past the newest message in this page.
		cursor = msgs[len(msgs)-1].ID
	}
	return persisted
}

// replayMessage re-infers every attachment of one already-posted bread message
// and persists the stats + discord info, WITHOUT sending a reply. For the
// reply-message fields it locates the bot's existing reply to this post (see
// findBotReply) so preview/jump links still resolve; if none is found those
// fields are left pointing at the original post. Returns attachments persisted.
func (b *Bot) replayMessage(original *discordgo.Message) int {
	saved, err := b.saveAttachments(original.Attachments)
	if err != nil {
		slog.Error("REPLAY HOTFIX: save attachments", "message_id", original.ID, "err", err)
		return 0
	}

	reply := b.findBotReply(original)
	// Fall back to the original post itself when no bot reply survives, so the
	// row still carries a usable id + jump url rather than nothing.
	replyMsg := reply
	if replyMsg == nil {
		replyMsg = original
	}

	ogID := mustParseID(original.ID)
	count := 0
	for _, sa := range saved {
		pred, outFile, err := b.inferAndSave(sa)
		if err != nil {
			slog.Error("REPLAY HOTFIX: inference", "message_id", original.ID, "file", sa.path, "err", err)
			continue
		}

		var roundness float64
		if pred.Roundness != nil {
			roundness = *pred.Roundness
		}
		var imageFilename string
		if pred.Image != nil {
			imageFilename = filepath.Base(outFile)
		}
		if err := b.db.UpsertMessageStats(ogID, sa.id, roundness, pred.Labels, imageFilename); err != nil {
			slog.Error("REPLAY HOTFIX: persist stats", "message_id", original.ID, "err", err)
			continue
		}
		if err := b.db.UpsertMessageDiscordInfo(
			ogID,
			sa.id,
			messageJumpURL(replyMsg),
			mustParseID(replyMsg.ID),
			mustParseID(original.Author.ID),
			mustParseID(original.ChannelID),
			mustParseID(original.GuildID),
			original.Timestamp.UnixMilli(),
		); err != nil {
			slog.Error("REPLAY HOTFIX: persist discord info", "message_id", original.ID, "err", err)
			continue
		}
		slog.Info("REPLAY HOTFIX: persisted",
			"message_id", original.ID, "attachment_id", sa.id,
			"roundness", roundness, "reply_found", reply != nil)
		count++
	}
	return count
}

// inferAndSave runs inference for one attachment and saves the annotated image
// (when the service returns one), returning the prediction and the file whose
// basename should be stored as image_filename. Mirrors the inference + image
// save half of sendBreadMessage, minus the cache/reply.
func (b *Bot) inferAndSave(att savedAttachment) (pred *inference.PredictResponse, outFile string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	p, err := b.inference.PredictFile(ctx, att.path, 0)
	if err != nil {
		return nil, "", err
	}
	outFile = att.path
	if p.Image != nil {
		outPath := filepath.Join(b.cfg.DownloadsPath, "predictions", filepath.Base(att.path))
		if err := p.SaveImage(outPath); err != nil {
			return nil, "", err
		}
		outFile = outPath
	}
	return p, outFile, nil
}

// findBotReply scans the messages immediately following the original post for
// the bot's own reply to it (the reply the normal pipeline would have sent).
// Returns nil if none is found within a small window. The bot always replies
// right after the post, so a shallow forward scan suffices.
func (b *Bot) findBotReply(original *discordgo.Message) *discordgo.Message {
	// after=original.ID → the messages posted just after it, newest-first.
	msgs, err := b.session.ChannelMessages(original.ChannelID, 20, "", original.ID, "")
	if err != nil {
		slog.Warn("REPLAY HOTFIX: scan for bot reply", "message_id", original.ID, "err", err)
		return nil
	}
	for _, msg := range msgs {
		if msg.Author == nil || msg.Author.ID != b.selfID {
			continue
		}
		if msg.MessageReference != nil && msg.MessageReference.MessageID == original.ID {
			return msg
		}
	}
	return nil
}

// parseOptionalID reads a snowflake id from an env var, returning 0 when unset
// or unparseable (open-ended bound).
func parseOptionalID(env string) int64 {
	v := os.Getenv(env)
	if v == "" {
		return 0
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		slog.Warn("REPLAY HOTFIX: ignoring unparseable bound", "env", env, "value", v)
		return 0
	}
	return id
}
