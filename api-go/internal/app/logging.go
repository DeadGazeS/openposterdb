// Package app contains the bootstrap logic for the openposterdb server:
// logging, database setup, schema application, background workers, and
// seeding. Keeping this out of package main leaves main() as thin wiring.
package app

import (
	"log/slog"
	"os"
	"strings"
)

// SetupLogging installs the process-wide slog text handler at the given
// level name ("debug", "info", "warn", "error", "off"; default "info").
// Values are sanitized the same way as config values: anything from the
// first space, tab, or "(" onward is ignored.
func SetupLogging(level string) {
	level = strings.TrimSpace(level)
	if i := strings.IndexAny(level, " \t("); i >= 0 {
		level = level[:i]
	}
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	case "off", "none":
		slogLevel = slog.Level(1000)
	default:
		slogLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slogLevel,
	})))
}
