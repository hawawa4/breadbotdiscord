package bot

import (
	"log/slog"
	"strconv"
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
