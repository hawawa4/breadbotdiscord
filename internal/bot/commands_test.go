package bot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseTopLimit(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantLimit  int
		wantSuffix string
	}{
		// The fixed behavior: n is the token after --top.
		{"--top 5", []string{"--top", "5"}, 5, ""},
		{"--top 1", []string{"--top", "1"}, 1, ""},
		// No n token at all defaults to 3 with no scold (bare $breadstats and
		// --top-alone are legitimate "just give me the default" invocations).
		{"no args", nil, 3, ""},
		{"empty args", []string{}, 3, ""},
		{"--top with no n", []string{"--top"}, 3, ""},
		{"--top clamps >10", []string{"--top", "50"}, 10, " (You're asking too much, nobody has seen a top 10 ever)"},
		{"--top exactly 10", []string{"--top", "10"}, 10, ""},
		{"--top invalid", []string{"--top", "abc"}, 3, " (You didn't enter a valid number. Shame on you)"},
		// "anything else" path: first arg is the n token.
		{"bare number", []string{"7"}, 7, ""},
		{"bare invalid word", []string{"foo"}, 3, " (You didn't enter a valid number. Shame on you)"},
		{"bare big number", []string{"99"}, 10, " (You're asking too much, nobody has seen a top 10 ever)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit, suffix := parseTopLimit(tc.args)
			if limit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tc.wantLimit)
			}
			if suffix != tc.wantSuffix {
				t.Errorf("suffix = %q, want %q", suffix, tc.wantSuffix)
			}
		})
	}
}

func TestTopLimitToken(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--top", "5"}, "5"},
		{[]string{"--top"}, ""},
		{[]string{"7"}, "7"},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := topLimitToken(tc.args); got != tc.want {
			t.Errorf("topLimitToken(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestTruncateForDiscord(t *testing.T) {
	// Short content passes through unchanged.
	if got := truncateForDiscord("hi"); got != "hi" {
		t.Errorf("short: got %q", got)
	}
	// Exactly at the limit is unchanged.
	exact := strings.Repeat("a", discordMaxMessageLen)
	if got := truncateForDiscord(exact); got != exact {
		t.Errorf("exact-limit content was modified (len %d)", len(got))
	}
	// Over the limit is clamped to <= the limit and ends with the ellipsis.
	long := strings.Repeat("a", discordMaxMessageLen+500)
	got := truncateForDiscord(long)
	if len(got) > discordMaxMessageLen {
		t.Errorf("truncated len = %d, want <= %d", len(got), discordMaxMessageLen)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated content missing ellipsis: %q", got[len(got)-6:])
	}
	// Truncating in the middle of a multi-byte rune must not split it.
	multibyte := strings.Repeat("é", discordMaxMessageLen)
	got = truncateForDiscord(multibyte)
	if len(got) > discordMaxMessageLen {
		t.Errorf("multibyte truncated len = %d, want <= %d", len(got), discordMaxMessageLen)
	}
	if !utf8.ValidString(got) {
		t.Errorf("multibyte truncation produced invalid UTF-8")
	}
}
