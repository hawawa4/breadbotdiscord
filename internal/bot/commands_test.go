package bot

import "testing"

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
		{"--top with no n", []string{"--top"}, 3, " (You didn't enter a valid number. Shame on you)"},
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
