package bot

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/hawawa4/breadbotdiscord/internal/db"
	"github.com/hawawa4/breadbotdiscord/internal/stats"
)

// dispatchCommand routes a parsed $-command to its handler. name is the command
// without the prefix; args are the space-split arguments after it.
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

// cmdBreadstats routes the $breadstats subcommands. Mirrors breadstats:
// no arg -> error; --history / --self have dedicated handlers; --top and
// anything else fall to the server leaderboard.
func (b *Bot) cmdBreadstats(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	if len(args) < 1 {
		b.reply(s, m, "Not enough arguments")
		return
	}
	switch args[0] {
	case "--history":
		b.breadstatsHistory(s, m)
	case "--self":
		b.breadstatsSelf(s, m)
	case "--top":
		b.breadstatsTop(s, m, args)
	default:
		b.breadstatsTop(s, m, args)
	}
}

// breadstatsSelf replies with the caller's min and max roundness plus jump
// URLs. Ports _breadstats_self; a user with no roundness rows reports 0% with
// no jump URL rather than crashing (the Python version would NameError).
func (b *Bot) breadstatsSelf(s *discordgo.Session, m *discordgo.MessageCreate) {
	authorID := mustParseID(m.Author.ID)

	var minPct, maxPct float64
	var minURL, maxURL string

	if msg, err := b.db.GetMinRoundnessForUser(authorID); err == nil {
		minPct = msg.Roundness.Float64 * 100
		minURL = msg.ReplyMessageJumpURL
	} else if !errors.Is(err, db.ErrUserNotFound) {
		slog.Error("breadstats self: min", "err", err)
	}
	if msg, err := b.db.GetMaxRoundnessForUser(authorID); err == nil {
		maxPct = msg.Roundness.Float64 * 100
		maxURL = msg.ReplyMessageJumpURL
	} else if !errors.Is(err, db.ErrUserNotFound) {
		slog.Error("breadstats self: max", "err", err)
	}

	content := fmt.Sprintf(`
                            Hello %s:
                            Min roundness:  %.2f%% on message: %s,
                            Max roundness %.2f%% on message: %s
                            `, m.Author.Username, minPct, minURL, maxPct, maxURL)
	b.reply(s, m, content)
}

// breadstatsTop replies with the best and worst leaderboards. Ports
// _breadstats_top WITH the confirmed bug fixes:
//   - parse the real n argument (the token after --top, or the first arg for
//     the "anything else" path), not the out-of-range args[2];
//   - label the worst list with the actual n instead of hardcoded "Worst 3".
func (b *Bot) breadstatsTop(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	limit, suffix := parseTopLimit(args)

	maxRows, err := b.db.GetMaxRoundnessLeaderboard(limit)
	if err != nil {
		slog.Error("breadstats top: max leaderboard", "err", err)
		return
	}
	minRows, err := b.db.GetMinRoundnessLeaderboard(limit)
	if err != nil {
		slog.Error("breadstats top: min leaderboard", "err", err)
		return
	}

	best := b.formatLeaderboard(fmt.Sprintf("Top %d%s:", limit, suffix), maxRows)
	worst := b.formatLeaderboard(fmt.Sprintf("Worst %d:", limit), minRows)
	b.reply(s, m, best+"\n"+worst)
}

// formatLeaderboard renders a leaderboard section: a header followed by one
// numbered line per message (author name, roundness %, jump URL). Mirrors the
// Python per-row formatting, including the "unknown" fallback for missing users.
func (b *Bot) formatLeaderboard(header string, rows []db.Message) string {
	var sb strings.Builder
	sb.WriteString(header)
	for i, msg := range rows {
		name := "unknown"
		if u, err := b.db.SelectUser(msg.AuthorID); err == nil {
			name = u.AuthorName
		}
		sb.WriteString(fmt.Sprintf("\n #%d: %s with %.2f%% on message %s",
			i+1, name, msg.Roundness.Float64*100, msg.ReplyMessageJumpURL))
	}
	return sb.String()
}

// parseTopLimit determines the leaderboard size and any joke suffix from the
// $breadstats args, applying the fixed parsing rules:
//   - the n token is the argument after "--top", or the first arg otherwise;
//   - a valid n > 10 clamps to 10 with the "asking too much" suffix;
//   - a missing/invalid n defaults to 3 with the "shame on you" suffix.
func parseTopLimit(args []string) (limit int, suffix string) {
	token := topLimitToken(args)
	n, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil {
		return 3, " (You didn't enter a valid number. Shame on you)"
	}
	if n > 10 {
		return 10, " (You're asking too much, nobody has seen a top 10 ever)"
	}
	return n, ""
}

// topLimitToken extracts the candidate n token: the argument following --top
// when present, otherwise the first argument. Empty if none.
func topLimitToken(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if args[0] == "--top" {
		if len(args) >= 2 {
			return args[1]
		}
		return ""
	}
	return args[0]
}

// breadstatsHistory renders the caller's roundness-history PNG and attaches it.
// Ports _breadstats_history.
func (b *Bot) breadstatsHistory(s *discordgo.Session, m *discordgo.MessageCreate) {
	authorID := mustParseID(m.Author.ID)
	history, err := b.db.GetRoundnessHistory(authorID)
	if err != nil {
		slog.Error("breadstats history: query", "err", err)
		return
	}

	savePath := filepath.Join(b.cfg.DownloadsPath, "plots", fmt.Sprintf("%d_roundhistory.png", authorID))
	points := make([]stats.RoundnessPoint, len(history))
	for i, p := range history {
		points[i] = stats.RoundnessPoint{Index: p.Index, Roundness: p.Roundness}
	}
	if err := stats.PlotRoundnessByUser(points, savePath); err != nil {
		slog.Error("breadstats history: plot", "err", err)
		return
	}

	if err := b.sendFile(s, m.Message, savePath, "Here's your graph with the roundness history"); err != nil {
		slog.Error("breadstats history: send", "err", err)
	}
}

// reply sends content as a reply to the triggering message, matching the
// Python `reference=ctx.message` behavior.
func (b *Bot) reply(s *discordgo.Session, m *discordgo.MessageCreate, content string) {
	if _, err := s.ChannelMessageSendReply(m.ChannelID, content, m.Reference()); err != nil {
		slog.Error("send reply", "channel", m.ChannelID, "err", err)
	}
}

// sendFile attaches a file as a reply to the target message.
func (b *Bot) sendFile(s *discordgo.Session, target *discordgo.Message, filePath, content string) error {
	_, err := b.sendFileReply(s, target, filePath, content)
	return err
}
