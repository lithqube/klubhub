package log

import (
	"os"

	"github.com/rs/zerolog"
)

// New creates a new zerolog.Logger writing JSON to stdout.
// The level parameter is parsed with zerolog.ParseLevel; if it is invalid,
// the logger defaults to InfoLevel.
func New(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	return zerolog.New(os.Stdout).
		Level(lvl).
		With().
		Timestamp().
		Logger()
}
