package bot

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestContainsID(t *testing.T) {
	ids := []int64{1, 2, 206734328879775746}
	if !containsID(ids, "206734328879775746") {
		t.Error("expected big id to be present")
	}
	if containsID(ids, "999") {
		t.Error("999 should not be present")
	}
	if containsID(ids, "not-a-number") {
		t.Error("malformed id should not match")
	}
}

func TestHasAllowedRole(t *testing.T) {
	allowed := []int64{100, 200}
	if !hasAllowedRole([]string{"50", "200"}, allowed) {
		t.Error("expected intersection on role 200")
	}
	if hasAllowedRole([]string{"50", "60"}, allowed) {
		t.Error("no intersection expected")
	}
	if hasAllowedRole(nil, allowed) {
		t.Error("empty roles should not match")
	}
}

func TestMessageJumpURL(t *testing.T) {
	m := &discordgo.Message{ID: "3", ChannelID: "2", GuildID: "1"}
	if got := messageJumpURL(m); got != "https://discord.com/channels/1/2/3" {
		t.Errorf("got %q", got)
	}
	dm := &discordgo.Message{ID: "3", ChannelID: "2"}
	if got := messageJumpURL(dm); got != "https://discord.com/channels/@me/2/3" {
		t.Errorf("DM url = %q", got)
	}
}
