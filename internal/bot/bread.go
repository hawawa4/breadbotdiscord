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
	if !containsID(b.cfg.DiscordBreadChannels, m.ChannelID) {
		return false
	}
	if m.Member == nil || !hasAllowedRole(m.Member.Roles, b.cfg.DiscordBreadRole) {
		return false
	}
	if len(m.Attachments) == 0 {
		return false
	}
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
	files, err := b.saveAttachments(m.Attachments)
	if err != nil {
		slog.Error("save attachments", "message_id", m.ID, "err", err)
		return
	}
	for _, f := range files {
		if err := b.sendBreadMessage(s, target, f, minConfidence); err != nil {
			slog.Error("send bread message", "message_id", target.ID, "file", f, "err", err)
		}
	}
}

// handleAreYouSure resolves the original bread message (reply → bot reply →
// original), then re-runs inference on THIS message's attachments at the
// override confidence, persisting against the original message id. Ports the
// areyousure branch of predict().
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

	files, err := b.saveAttachments(m.Attachments)
	if err != nil {
		slog.Error("save attachments", "message_id", m.ID, "err", err)
		return
	}
	for _, f := range files {
		if err := b.sendBreadMessage(s, original, f, b.cfg.OverrideDetectionConfidence); err != nil {
			slog.Error("send bread message (retry)", "message_id", original.ID, "file", f, "err", err)
		}
	}
}

// sendBreadMessage runs the compute tree for one file, sends the reply (image +
// comment) to the target message's channel, and persists the results against
// the target message id. Ports _send_bread_message.
func (b *Bot) sendBreadMessage(s *discordgo.Session, target *discordgo.Message, inputFile string, minConfidence float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	// Typing indicator during inference (mirrors the `async with typing()`).
	_ = s.ChannelTyping(target.ChannelID)

	outFile, comment, pred, err := b.computeBreadMessage(ctx, inputFile, minConfidence)
	if err != nil {
		return err
	}

	sent, err := b.sendFileReply(s, target, outFile, comment)
	if err != nil {
		return fmt.Errorf("send file reply: %w", err)
	}

	// Persist inference results (roundness may be nil → stored as-is).
	var roundness float64
	if pred.Roundness != nil {
		roundness = *pred.Roundness
	}
	ogID := mustParseID(target.ID)
	if err := b.db.UpsertMessageStats(ogID, roundness, pred.Labels); err != nil {
		return fmt.Errorf("persist stats: %w", err)
	}
	if err := b.db.UpsertMessageDiscordInfo(
		ogID,
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

// computeBreadMessage calls inference and applies the response decision tree,
// returning the file to attach, the comment, and the raw prediction. Ports
// compute_bread_message_for_file.
//
// Note: the "is it bread" gate compares against the configured
// BreadDetectionConfidence (0.5), NOT minConfidence — minConfidence only gates
// which per-label sentiments are appended. This matches the Python exactly.
func (b *Bot) computeBreadMessage(ctx context.Context, inputFile string, minConfidence float64) (outFile, comment string, pred *inference.PredictResponse, err error) {
	pred, err = b.inference.PredictFile(ctx, inputFile)
	if err != nil {
		return "", "", nil, fmt.Errorf("inference: %w", err)
	}

	breadConf, hasBread := pred.Labels["bread"]
	if !hasBread {
		return inputFile, "This isn't bread at all!", pred, nil
	}
	if breadConf <= b.cfg.BreadDetectionConfidence {
		return inputFile, "This is only very mildly bread. Metaphysical bread even.", pred, nil
	}

	labelsComment := messageContentFromLabels(toLabels(pred.OrderedLabels()), minConfidence)
	if pred.Image != nil {
		outPath := filepath.Join(b.cfg.DownloadsPath, "predictions", filepath.Base(inputFile))
		if err := pred.SaveImage(outPath); err != nil {
			return "", "", nil, fmt.Errorf("save annotated image: %w", err)
		}
		return outPath, labelsComment + messageFromRoundness(pred.Roundness), pred, nil
	}
	return inputFile, labelsComment + ". I couldn't find the shape dough. (Get it? Though - dough ehehehehe)", pred, nil
}

// saveAttachments downloads each attachment to the downloads dir and returns
// the saved file paths. Ports the save_attachment gather.
func (b *Bot) saveAttachments(attachments []*discordgo.MessageAttachment) ([]string, error) {
	if err := os.MkdirAll(b.cfg.DownloadsPath, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, a := range attachments {
		dest := filepath.Join(b.cfg.DownloadsPath, a.Filename)
		if err := downloadFile(a.URL, dest); err != nil {
			return nil, fmt.Errorf("download %q: %w", a.Filename, err)
		}
		paths = append(paths, dest)
	}
	return paths, nil
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
	return s.ChannelMessageSendComplex(target.ChannelID, &discordgo.MessageSend{
		Content:   content,
		Files:     []*discordgo.File{{Name: filepath.Base(filePath), Reader: f}},
		Reference: &discordgo.MessageReference{MessageID: target.ID, ChannelID: target.ChannelID, GuildID: target.GuildID},
	})
}

// toLabels converts inference OrderedLabels to the responses.Label type.
func toLabels(in []inference.OrderedLabel) []Label {
	out := make([]Label, len(in))
	for i, l := range in {
		out[i] = Label{Name: l.Name, Confidence: l.Confidence}
	}
	return out
}
