package bot

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/bwmarrin/discordgo"
)

// mustParseID converts a Discord snowflake string to int64 for DB storage.
// Discord ids always fit in int64; a parse failure indicates malformed gateway
// data, so we log and return 0 rather than crash the message handler.
func mustParseID(s string) int64 {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		slog.Error("parse discord id", "value", s, "err", err)
		return 0
	}
	return id
}

// idString renders an int64 snowflake (as stored in config/DB) back to the
// string form the discordgo REST API expects.
func idString(id int64) string {
	return strconv.FormatInt(id, 10)
}

// containsID reports whether the snowflake string id is in the int64 id list.
func containsID(ids []int64, id string) bool {
	parsed, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return false
	}
	for _, v := range ids {
		if v == parsed {
			return true
		}
	}
	return false
}

// hasAllowedRole reports whether any of the member's role ids (snowflake
// strings) is in the allowed int64 list. Mirrors the set-intersection check in
// is_bread_candidate.
func hasAllowedRole(memberRoles []string, allowed []int64) bool {
	for _, r := range memberRoles {
		if containsID(allowed, r) {
			return true
		}
	}
	return false
}

// messageJumpURL builds the canonical Discord jump URL for a message, which
// discordgo does not expose directly. Matches discord.py's Message.jump_url.
func messageJumpURL(m *discordgo.Message) string {
	guild := m.GuildID
	if guild == "" {
		guild = "@me"
	}
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guild, m.ChannelID, m.ID)
}
