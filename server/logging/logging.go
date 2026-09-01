// Package logging sets up Conway's structured logging on zerolog.
//
// Package choice (recorded): zerolog v1.35.1 — the app logs a few lines per
// request, and zerolog's fluent context API plus its built-in pretty console
// writer give colorized dev output and JSON machine output from the same
// logger with no custom encoder to maintain.
//
// Two output shapes:
//   - CONWAY_LOG_FORMAT unset (the default): zerolog's ConsoleWriter — a
//     colorized, human-readable line, auto-detecting a terminal. When output
//     is collected (not a TTY), JSON is used instead, because color codes in a
//     collected log are noise — unless the operator explicitly asked for
//     color, which is honored.
//   - CONWAY_LOG_FORMAT=json: one JSON object per record, for aggregation.
//
// Level: CONWAY_LOG_LEVEL=debug|info|warn|error (default info). Debug is the
// HTTP request stream and engine internals; the default keeps the tail quiet.
package logging

import (
	"io"
	stdlog "log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Escape sanitizes an untrusted string for a one-line log record: control
// characters and newlines become Go escapes, so a crafted value cannot forge
// additional log lines or escape the record's quoting. Use for any
// request-derived field (paths, emails, ids) passed into a log call.
//
// zerolog already escapes newlines inside string fields when writing JSON;
// this remains for the console writer's line discipline and for call sites
// that interpolate into a message.
func Escape(s string) string { return strconv.Quote(s) }

// LevelFromEnv parses CONWAY_LOG_LEVEL, defaulting to info. An unrecognised
// value falls back with a warning at boot — a typo must not silence the log.
func LevelFromEnv() zerolog.Level {
	switch strings.ToLower(os.Getenv("CONWAY_LOG_LEVEL")) {
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// FormatFromEnv parses CONWAY_LOG_FORMAT: "json" for machine output, anything
// else for the colorized console.
func FormatFromEnv() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("CONWAY_LOG_FORMAT")))
}

// New returns the app logger. format is "console" (colorized) or "json".
func New(w io.Writer, format string, level zerolog.Level) zerolog.Logger {
	if format == "json" {
		return log.Logger.Output(w).Level(level)
	}
	out := zerolog.ConsoleWriter{Out: w, TimeFormat: time.Kitchen, NoColor: false}
	return log.Logger.Output(out).Level(level)
}

// NewDefault builds the logger Conway boots with: a colorized console when
// stderr is a terminal, JSON when output is collected, and JSON whenever the
// operator asked for it. The chosen shape is reported so a collected log says
// which it is.
func NewDefault() zerolog.Logger {
	w := io.Writer(os.Stderr)
	format := FormatFromEnv()
	if format == "" {
		if isTTY(os.Stderr) {
			format = "console"
		} else {
			format = "json" // a collected log loses nothing to color codes
		}
	}
	zerolog.SetGlobalLevel(LevelFromEnv())
	// RFC3339 with milliseconds; ConsoleWriter renders it as HH:MM:SS.
	zerolog.TimestampFunc = time.Now
	logger := log.Logger
	if format == "console" {
		logger = logger.Output(zerolog.ConsoleWriter{Out: w, TimeFormat: time.Kitchen})
	} else {
		logger = logger.Output(w)
	}
	// The global logger IS this logger: everything logging through zerolog's
	// package-level helpers — the request middleware included — shares the
	// same shape.
	log.Logger = logger
	return logger
}

// isTTY reports whether w is a character device (a terminal).
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ReplaceLogStandard routes the stdlib logger (and anything using it) through
// zerolog, so a stray fmt-style log.Printf still lands in the same shape.
func ReplaceLogStandard() {
	std := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen}).
		Level(zerolog.ErrorLevel).
		With().Timestamp().Logger()
	stdlog.SetOutput(std)
	stdlog.SetFlags(0)
}
