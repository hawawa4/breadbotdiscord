// Package config loads BreadBot configuration from environment variables.
//
// This replaces the Python pydantic-settings module (src/settings.py). We keep
// the same env var names but drop the odd env_prefix="__" quirk: names are read
// unprefixed (e.g. DISCORD_TOKEN, not __DISCORD_TOKEN).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration.
type Config struct {
	Debug bool

	// Confidence thresholds for the inference microservice.
	BreadDetectionConfidence    float64
	OverrideDetectionConfidence float64

	DiscordToken         string
	DiscordBreadChannels []int64
	DiscordBreadRole     []int64

	DBDataPath    string
	DownloadsPath string

	InferenceServiceURL string

	// AdminAPIToken guards the read-only HTTP server. Empty = auth disabled.
	AdminAPIToken string
	// AdminAPIAddr is the listen address for the read-only HTTP server.
	AdminAPIAddr string
}

// Load reads configuration from the environment, applying defaults that match
// the Python implementation. DISCORD_TOKEN is required; everything else has a
// sensible default.
func Load() (*Config, error) {
	c := &Config{
		Debug:                       envBool("DEBUG", false),
		BreadDetectionConfidence:    envFloat("BREAD_DETECTION_CONFIDENCE", 0.5),
		OverrideDetectionConfidence: envFloat("OVERRIDE_DETECTION_CONFIDENCE", 0.05),
		DiscordToken:                os.Getenv("DISCORD_TOKEN"),
		DBDataPath:                  envStr("DB_DATA_PATH", "dbdata/messages.db"),
		DownloadsPath:               envStr("DOWNLOADS_PATH", "downloads/"),
		InferenceServiceURL:         envStr("INFERENCE_SERVICE_URL", "http://localhost:8000"),
		AdminAPIToken:               os.Getenv("ADMIN_API_TOKEN"),
		AdminAPIAddr:                envStr("ADMIN_API_ADDR", ":8080"),
	}

	if c.DiscordToken == "" {
		return nil, fmt.Errorf("config: DISCORD_TOKEN is required")
	}

	channels, err := parseIntList(os.Getenv("DISCORD_BREAD_CHANNELS"))
	if err != nil {
		return nil, fmt.Errorf("config: DISCORD_BREAD_CHANNELS: %w", err)
	}
	c.DiscordBreadChannels = channels

	roles, err := parseIntList(os.Getenv("DISCORD_BREAD_ROLE"))
	if err != nil {
		return nil, fmt.Errorf("config: DISCORD_BREAD_ROLE: %w", err)
	}
	c.DiscordBreadRole = roles

	return c, nil
}

// parseIntList parses the Python-style "[1,2,3]" list format into int64 ids.
// An empty or whitespace-only string yields an empty (non-nil) slice.
func parseIntList(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "[]")
	out := []int64{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", part, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	// Match pydantic's permissive bool parsing.
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "y", "t":
		return true
	case "0", "false", "no", "off", "n", "f":
		return false
	default:
		return def
	}
}

func envFloat(key string, def float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return def
	}
	return f
}
