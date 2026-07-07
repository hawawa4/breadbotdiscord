package bot

import (
	"reflect"
	"testing"
)

func TestParseCommand(t *testing.T) {
	var b Bot
	cases := []struct {
		name     string
		content  string
		wantOK   bool
		wantName string
		wantArgs []string
	}{
		{"hello", "$hello", true, "hello", []string{}},
		{"breadstats no arg", "$breadstats", true, "breadstats", []string{}},
		{"breadstats top n", "$breadstats --top 5", true, "breadstats", []string{"--top", "5"}},
		{"leading/trailing space", "  $hello  ", true, "hello", []string{}},
		{"extra inner spaces", "$breadstats   --self", true, "breadstats", []string{"--self"}},
		{"unknown command falls through", "$frobnicate", false, "", nil},
		{"plain text", "just a normal message", false, "", nil},
		{"prefix only", "$", false, "", nil},
		{"empty", "", false, "", nil},
		{"not at start", "hey $hello", false, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, args, ok := b.parseCommand(tc.content)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %v, want %v", args, tc.wantArgs)
			}
		})
	}
}

func TestIsRegisteredCommand(t *testing.T) {
	for _, name := range []string{"hello", "breadstats"} {
		if !isRegisteredCommand(name) {
			t.Errorf("%q should be registered", name)
		}
	}
	for _, name := range []string{"", "Hello", "top", "frobnicate"} {
		if isRegisteredCommand(name) {
			t.Errorf("%q should not be registered", name)
		}
	}
}

func TestMustParseID(t *testing.T) {
	if got := mustParseID("206734328879775746"); got != 206734328879775746 {
		t.Errorf("got %d, want 206734328879775746", got)
	}
	if got := mustParseID("not-a-number"); got != 0 {
		t.Errorf("malformed id should yield 0, got %d", got)
	}
}
