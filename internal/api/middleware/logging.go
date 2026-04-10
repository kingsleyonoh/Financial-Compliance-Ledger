package middleware

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// NewLogger creates a structured zerolog.Logger configured for the given level.
// Valid levels: "debug", "info", "warn", "error". Defaults to "info".
// In development, output is formatted with console writer for readability.
func NewLogger(level string) zerolog.Logger {
	lvl := parseLevel(level)

	return zerolog.New(
		zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		},
	).Level(lvl).With().Timestamp().Logger()
}

// NewProductionLogger creates a JSON-formatted zerolog.Logger for production use.
func NewProductionLogger(level string) zerolog.Logger {
	lvl := parseLevel(level)

	return zerolog.New(os.Stdout).
		Level(lvl).
		With().
		Timestamp().
		Logger()
}

// parseLevel converts a string log level to a zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
