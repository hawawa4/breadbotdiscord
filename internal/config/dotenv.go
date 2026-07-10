package config

import (
	"bufio"
	"log/slog"
	"os"
	"strings"
)

// LoadDotEnv reads a .env file (KEY=value lines) and sets any variables not
// already present in the environment. This mirrors pydantic-settings' env_file
// support for local development; real environments (e.g. docker-compose's
// env_file) set vars directly and take precedence.
//
// The .env file is purely a convenience: if it is missing OR unreadable (e.g. a
// permission mismatch when running in a container as a non-root user), it is
// skipped without error — configuration then comes entirely from the process
// environment.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			slog.Debug("skipping .env", "path", path, "reason", err)
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip optional surrounding quotes.
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
