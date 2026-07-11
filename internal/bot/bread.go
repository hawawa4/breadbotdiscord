package bot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/hawawa4/breadbotdiscord/internal/inference"
)

// areYouSureTriggers are the substrings that mark an "are you sure" retry.
var areYouSureTriggers = []string{"are you sure", "no way"}

// onPlainMessage handles non-command messages: the bread-detection pipeline.
// It runs the whole thing under a recover-style guard so one failure logs and
// never crashes the handler, matching the Python broad try/except.
func (b *Bot) onPlainMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("plain message panic", "message_id", m.ID, "recover", r)
		}
	}()

	switch {
	case b.isBreadCandidate(m):
		b.handleBreadCandidate(s, m, m.Message, b.cfg.BreadDetectionConfidence)
	case b.isAreYouSureMessage(m):
		slog.Debug("are-you-sure message; re-running inference", "message_id", m.ID)
		b.handleAreYouSure(s, m)
	}
}

// isBreadCandidate reports whether a message qualifies for bread detection:
// posted in an allowed channel, by an author with an allowed role, with at
// least one attachment. Ports is_bread_candidate.
func (b *Bot) isBreadCandidate(m *discordgo.MessageCreate) bool {
	// Only bother diagnosing messages that actually carry an attachment —
	// otherwise every plain chat line would log a rejection.
	diag := len(m.Attachments) > 0

	if !containsID(b.cfg.DiscordBreadChannels, m.ChannelID) {
		if diag {
			slog.Info("bread candidate rejected: channel not in DISCORD_BREAD_CHANNELS",
				"channel", m.ChannelID, "allowed_channels", b.cfg.DiscordBreadChannels)
		}
		return false
	}
	if m.Member == nil {
		if diag {
			slog.Info("bread candidate rejected: message has no Member (roles unknown)",
				"message_id", m.ID, "author", m.Author.Username)
		}
		return false
	}
	if !hasAllowedRole(m.Member.Roles, b.cfg.DiscordBreadRole) {
		if diag {
			slog.Info("bread candidate rejected: author lacks an allowed role",
				"author", m.Author.Username, "author_roles", m.Member.Roles, "allowed_roles", b.cfg.DiscordBreadRole)
		}
		return false
	}
	if len(m.Attachments) == 0 {
		return false
	}
	slog.Info("bread candidate accepted", "message_id", m.ID, "channel", m.ChannelID, "attachments", len(m.Attachments))
	return true
}

// isAreYouSureMessage reports whether a message is a reply to one of the bot's
// own messages and contains a trigger phrase. Ports is_areyousure_message.
func (b *Bot) isAreYouSureMessage(m *discordgo.MessageCreate) bool {
	if m.MessageReference == nil || m.ReferencedMessage == nil {
		return false
	}
	if m.ReferencedMessage.Author == nil || m.ReferencedMessage.Author.ID != b.selfID {
		return false
	}
	content := strings.ToLower(m.Content)
	for _, trigger := range areYouSureTriggers {
		if strings.Contains(content, trigger) {
			return true
		}
	}
	return false
}

// handleBreadCandidate downloads each attachment and runs the send-bread flow
// against the given message, at the given confidence.
func (b *Bot) handleBreadCandidate(s *discordgo.Session, m *discordgo.MessageCreate, target *discordgo.Message, minConfidence float64) {
	saved, err := b.saveAttachments(m.Attachments)
	if err != nil {
		slog.Error("save attachments", "message_id", m.ID, "err", err)
		return
	}
	for _, sa := range saved {
		if err := b.sendBreadMessage(s, target, sa, minConfidence); err != nil {
			slog.Error("send bread message", "message_id", target.ID, "file", sa.path, "err", err)
		}
	}
}

// handleAreYouSure resolves the original bread message (reply → bot reply →
// original) and re-renders its verdict at the relaxed override confidence so
// every label is mentioned. It first tries the in-memory prediction cache
// (populated by the normal path): on a hit it reuses the cached full response
// AND annotated image — no second inference call. On a miss (e.g. after a
// restart or eviction) it falls back to a fresh inference run on the original
// post's attachments. Ports the areyousure branch of predict().
func (b *Bot) handleAreYouSure(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Timeline: user's bread post -> bot reply -> user reply to bot reply.
	// The bot reply (m.ReferencedMessage) itself references the original post.
	botReply := m.ReferencedMessage
	if botReply.MessageReference == nil {
		slog.Warn("are-you-sure: bot reply has no reference", "message_id", m.ID)
		return
	}
	ref := botReply.MessageReference
	original, err := s.ChannelMessage(ref.ChannelID, ref.MessageID)
	if err != nil {
		slog.Error("are-you-sure: resolve original message", "err", err)
		return
	}
	// A message fetched via REST (ChannelMessage) has an empty GuildID, unlike
	// one delivered over the gateway. Backfill it from the reference, then from
	// the triggering message (both are gateway objects and share the guild), so
	// persistence doesn't try to parse an empty id.
	if original.GuildID == "" {
		original.GuildID = ref.GuildID
	}
	if original.GuildID == "" {
		original.GuildID = m.GuildID
	}
	ogID := mustParseID(original.ID)

	// The original post may carry several images; each is a distinct cache entry
	// keyed by (message id, attachment id). Re-render every image whose
	// prediction is still cached, and remember which we handled so the fallback
	// only re-infers the ones that missed.
	attachments := original.Attachments
	if len(attachments) == 0 {
		slog.Warn("are-you-sure: original has no attachments; nothing to re-run", "message_id", m.ID)
		return
	}

	handled := make(map[int64]bool, len(attachments))
	for _, a := range attachments {
		attID := mustParseID(a.ID)
		cached, ok := b.predCache.get(predKey{ogMessageID: ogID, attachmentID: attID})
		if !ok {
			continue
		}
		slog.Info("are-you-sure: cache hit; re-rendering at relaxed confidence",
			"original_message_id", original.ID, "attachment_id", a.ID,
			"override_confidence", b.cfg.OverrideDetectionConfidence)
		outFile, comment := renderBreadMessage(cached.outFile, cached.inFile, cached.pred, b.cfg.OverrideDetectionConfidence)
		if err := b.deliverBreadMessage(s, original, attID, outFile, comment, cached.pred); err != nil {
			slog.Error("are-you-sure: deliver from cache", "message_id", original.ID, "err", err)
		}
		handled[attID] = true
	}

	// Any attachment not served from cache (restart/eviction) is re-inferred
	// from the ORIGINAL post's attachments — the "are you sure" reply itself is
	// usually just text.
	var missed []*discordgo.MessageAttachment
	for _, a := range attachments {
		if !handled[mustParseID(a.ID)] {
			missed = append(missed, a)
		}
	}
	if len(missed) == 0 {
		return
	}
	slog.Info("are-you-sure: cache miss; re-running inference at relaxed confidence",
		"original_message_id", original.ID,
		"override_confidence", b.cfg.OverrideDetectionConfidence,
		"attachments", len(missed),
	)
	saved, err := b.saveAttachments(missed)
	if err != nil {
		slog.Error("save attachments", "message_id", m.ID, "err", err)
		return
	}
	for _, sa := range saved {
		if err := b.sendBreadMessage(s, original, sa, b.cfg.OverrideDetectionConfidence); err != nil {
			slog.Error("send bread message (retry)", "message_id", original.ID, "file", sa.path, "err", err)
		}
	}
}

// sendBreadMessage runs inference for one attachment, caches the full response,
// renders + sends the reply, and persists the results against the target
// message id + attachment id. Ports _send_bread_message.
func (b *Bot) sendBreadMessage(s *discordgo.Session, target *discordgo.Message, att savedAttachment, minConfidence float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	// Typing indicator during inference (mirrors the `async with typing()`).
	_ = s.ChannelTyping(target.ChannelID)

	// Always ask the service for everything (threshold 0); we filter here.
	pred, err := b.inference.PredictFile(ctx, att.path, 0)
	if err != nil {
		return fmt.Errorf("inference: %w", err)
	}

	// Save the annotated image once (if any) so the cache can reuse it. The
	// input basename is already collision-free (attachment-id prefixed), so the
	// annotated file inherits a unique name too.
	outFile := att.path
	if pred.Image != nil {
		outPath := filepath.Join(b.cfg.DownloadsPath, "predictions", filepath.Base(att.path))
		if err := pred.SaveImage(outPath); err != nil {
			return fmt.Errorf("save annotated image: %w", err)
		}
		outFile = outPath
	}

	ogID := mustParseID(target.ID)
	b.predCache.put(predKey{ogMessageID: ogID, attachmentID: att.id},
		cachedPrediction{pred: pred, outFile: outFile, inFile: att.path})

	renderedFile, comment := renderBreadMessage(outFile, att.path, pred, minConfidence)
	return b.deliverBreadMessage(s, target, att.id, renderedFile, comment, pred)
}

// deliverBreadMessage sends the reply and persists the inference results against
// the target message id + attachment id. Shared by the fresh path and the
// cache-rerender path.
func (b *Bot) deliverBreadMessage(s *discordgo.Session, target *discordgo.Message, attachmentID int64, outFile, comment string, pred *inference.PredictResponse) error {
	sent, err := b.sendFileReply(s, target, outFile, comment)
	if err != nil {
		return fmt.Errorf("send file reply: %w", err)
	}

	// Persist inference results (roundness may be nil → stored as 0).
	var roundness float64
	if pred.Roundness != nil {
		roundness = *pred.Roundness
	}
	// The annotated PNG is saved under downloads/predictions/ only when the
	// service returned an image; outFile is that annotated path in exactly that
	// case (see renderBreadMessage). Persist its basename so the HTTP API/gallery
	// can link the message to its image; leave empty (→ NULL) otherwise.
	var imageFilename string
	if pred.Image != nil {
		imageFilename = filepath.Base(outFile)
	}
	ogID := mustParseID(target.ID)
	if err := b.db.UpsertMessageStats(ogID, attachmentID, roundness, pred.Labels, imageFilename); err != nil {
		return fmt.Errorf("persist stats: %w", err)
	}
	if err := b.db.UpsertMessageDiscordInfo(
		ogID,
		attachmentID,
		messageJumpURL(sent),
		mustParseID(sent.ID),
		mustParseID(target.Author.ID),
		mustParseID(target.ChannelID),
		mustParseID(target.GuildID),
	); err != nil {
		return fmt.Errorf("persist discord info: %w", err)
	}
	return nil
}

// renderBreadMessage applies the response decision tree to an ALREADY-OBTAINED
// prediction, returning the file to attach and the comment. Ports the
// non-inference half of compute_bread_message_for_file.
//
// annotatedFile is the saved annotated image (when pred.Image != nil) and
// inputFile is the plain source image; renderBreadMessage picks between them.
//
// minConfidence is the relaxable threshold: it gates BOTH the "is it bread"
// decision and which per-label sentiments are appended. On the normal path it
// is BreadDetectionConfidence (0.5); on an "are you sure" retry it is the lower
// OverrideDetectionConfidence, so a marginal bread the user is sure about
// actually passes the gate and gets the full treatment (the Python version
// only relaxed the label sentiments, not the gate, so the retry did nothing —
// this fixes that).
func renderBreadMessage(annotatedFile, inputFile string, pred *inference.PredictResponse, minConfidence float64) (outFile, comment string) {
	breadConf, hasBread := pred.Labels["bread"]
	if !hasBread {
		return inputFile, "This isn't bread at all!"
	}
	if breadConf <= minConfidence {
		return inputFile, "This is only very mildly bread. Metaphysical bread even."
	}

	labelsComment := messageContentFromLabels(toLabels(pred.OrderedLabels()), minConfidence)
	if pred.Image != nil {
		return annotatedFile, labelsComment + messageFromRoundness(pred.Roundness)
	}
	return inputFile, labelsComment + ". I couldn't find the shape dough. (Get it? Though - dough ehehehehe)"
}

// savedAttachment is one downloaded attachment: its local file path plus the
// Discord attachment id, which disambiguates the several images of a single
// message throughout the pipeline (cache key, DB composite key).
type savedAttachment struct {
	path string
	id   int64
}

// saveAttachments downloads each attachment to the downloads dir and returns
// the saved files paired with their attachment ids. Ports the save_attachment
// gather.
//
// Each file is named "{attachmentID}_{filename}" rather than the bare
// attachment filename. Discord filenames are NOT unique — a message can carry
// several attachments all named "image.png" (screenshots/pastes always are),
// and different messages reuse the same names constantly. The bare-name scheme
// meant later attachments clobbered earlier ones on disk (so only the last of N
// images was ever inferred/saved) and cross-message collisions overwrote each
// other's annotated predictions. The attachment ID is a globally unique
// snowflake, so prefixing with it makes every saved file distinct while keeping
// the original name readable. The annotated prediction file inherits this
// unique basename, and so does the DB's image_filename key.
func (b *Bot) saveAttachments(attachments []*discordgo.MessageAttachment) ([]savedAttachment, error) {
	if err := os.MkdirAll(b.cfg.DownloadsPath, 0o755); err != nil {
		return nil, err
	}
	var saved []savedAttachment
	for _, a := range attachments {
		dest := filepath.Join(b.cfg.DownloadsPath, attachmentFilename(a))
		if err := downloadFile(a.URL, dest); err != nil {
			return nil, fmt.Errorf("download %q: %w", a.Filename, err)
		}
		saved = append(saved, savedAttachment{path: dest, id: mustParseID(a.ID)})
	}
	return saved, nil
}

// attachmentFilename builds a collision-free, path-safe local filename for an
// attachment: "{id}_{sanitized filename}". The id disambiguates; sanitizing the
// filename strips any path separators so a hostile/odd filename can't escape
// the downloads dir. Falls back to a plain name if the id is somehow empty.
func attachmentFilename(a *discordgo.MessageAttachment) string {
	name := sanitizeFilename(a.Filename)
	if a.ID == "" {
		return name
	}
	return a.ID + "_" + name
}

// sanitizeFilename reduces a filename to a single safe path segment: it takes
// the base name (dropping any directory parts) and replaces the remaining path
// separators. An empty result becomes "file".
func sanitizeFilename(name string) string {
	name = filepath.Base(filepath.FromSlash(name))
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

// downloadFile fetches url and writes the body to dest.
func downloadFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// sendFileReply sends a file with a comment as a reply to target.
func (b *Bot) sendFileReply(s *discordgo.Session, target *discordgo.Message, filePath, content string) (*discordgo.Message, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sent, err := s.ChannelMessageSendComplex(target.ChannelID, &discordgo.MessageSend{
		Content:   content,
		Files:     []*discordgo.File{{Name: filepath.Base(filePath), Reader: f}},
		Reference: &discordgo.MessageReference{MessageID: target.ID, ChannelID: target.ChannelID, GuildID: target.GuildID},
	})
	if err != nil {
		return nil, err
	}
	slog.Info("replied to message",
		"to_message_id", target.ID,
		"channel", target.ChannelID,
		"kind", "file",
		"file", filepath.Base(filePath),
		"content_len", len(content),
	)
	return sent, nil
}

// toLabels converts inference OrderedLabels to the responses.Label type.
func toLabels(in []inference.OrderedLabel) []Label {
	out := make([]Label, len(in))
	for i, l := range in {
		out[i] = Label{Name: l.Name, Confidence: l.Confidence}
	}
	return out
}
