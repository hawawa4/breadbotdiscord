package config

import (
	"reflect"
	"testing"
)

func TestParseIntList(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []int64
	}{
		{"empty", "", []int64{}},
		{"brackets only", "[]", []int64{}},
		{"single", "[123]", []int64{123}},
		{"multi", "[1,2,3]", []int64{1, 2, 3}},
		{"spaces", "[ 1, 2 , 3 ]", []int64{1, 2, 3}},
		{"no brackets", "1,2,3", []int64{1, 2, 3}},
		{"trailing comma", "[1,2,]", []int64{1, 2}},
		{"big ids", "[206734328879775746]", []int64{206734328879775746}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIntList(tc.in)
			if err != nil {
				t.Fatalf("parseIntList(%q) error: %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseIntList(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"/":              "",
		"breadbot":       "/breadbot",
		"/breadbot":      "/breadbot",
		"/breadbot/":     "/breadbot",
		"  /breadbot/  ": "/breadbot",
		"app/breadbot":   "/app/breadbot",
		"/app/breadbot/": "/app/breadbot",
	}
	for in, want := range cases {
		if got := normalizeBasePath(in); got != want {
			t.Errorf("normalizeBasePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseIntListInvalid(t *testing.T) {
	if _, err := parseIntList("[1,x,3]"); err == nil {
		t.Error("expected error for non-numeric id, got nil")
	}
}

func TestLoadRequiresToken(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Error("expected error when DISCORD_TOKEN is unset")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "tok")
	t.Setenv("DISCORD_BREAD_CHANNELS", "[1,2]")
	t.Setenv("DISCORD_BREAD_ROLE", "[9]")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if c.BreadDetectionConfidence != 0.5 {
		t.Errorf("BreadDetectionConfidence = %v, want 0.5", c.BreadDetectionConfidence)
	}
	if c.OverrideDetectionConfidence != 0.05 {
		t.Errorf("OverrideDetectionConfidence = %v, want 0.05", c.OverrideDetectionConfidence)
	}
	if c.DBDataPath != "dbdata/messages.db" {
		t.Errorf("DBDataPath = %q, want dbdata/messages.db", c.DBDataPath)
	}
	if c.DownloadsPath != "downloads/" {
		t.Errorf("DownloadsPath = %q, want downloads/", c.DownloadsPath)
	}
	if c.InferenceServiceURL != "http://localhost:8000" {
		t.Errorf("InferenceServiceURL = %q, want http://localhost:8000", c.InferenceServiceURL)
	}
	if !reflect.DeepEqual(c.DiscordBreadChannels, []int64{1, 2}) {
		t.Errorf("DiscordBreadChannels = %v, want [1 2]", c.DiscordBreadChannels)
	}
	if !reflect.DeepEqual(c.DiscordBreadRole, []int64{9}) {
		t.Errorf("DiscordBreadRole = %v, want [9]", c.DiscordBreadRole)
	}
	if c.CatchUpLimit != 50 {
		t.Errorf("CatchUpLimit = %d, want 50", c.CatchUpLimit)
	}
}

func TestEnvInt(t *testing.T) {
	cases := map[string]int{"": 7, "10": 10, "0": 0, "notnum": 7}
	for in, want := range cases {
		t.Setenv("CATCH_UP_LIMIT", in)
		if got := envInt("CATCH_UP_LIMIT", 7); got != want {
			t.Errorf("envInt(CATCH_UP_LIMIT=%q) = %d, want %d", in, got, want)
		}
	}
}

func TestEnvBool(t *testing.T) {
	cases := map[string]bool{"true": true, "1": true, "yes": true, "false": false, "0": false, "off": false}
	for in, want := range cases {
		t.Setenv("DEBUG", in)
		if got := envBool("DEBUG", false); got != want {
			t.Errorf("envBool(DEBUG=%q) = %v, want %v", in, got, want)
		}
	}
}
