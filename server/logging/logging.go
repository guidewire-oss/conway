// Package logging sets up Conway's structured logging (spec: observability).
//
// One choice is deliberate: stdlib log/slog with ZERO third-party
// dependencies — the colorized console handler is written here (~100 lines)
// rather than pulled in (tint et al), because the app's dependency budget is
// a stated value and a colorized handler is a closed, small problem.
//
// Two output shapes:
//   - CONWAY_LOG_FORMAT=color (the default): a colorized console line, one per
//     record — level colored, time dimmed, message plain, attributes dimmed.
//     Built for a human tailing `docker compose logs` or `make logs`.
//   - json: one JSON object per record, for log aggregation. Docker users set
//     CONWAY_LOG_FORMAT=json and every field stays queryable.
//
// Level: CONWAY_LOG_LEVEL=debug|info|warn|error (default info). Debug is the
// engine's internals; production defaults keep the tail readable.
package logging

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// ANSI escapes. Only emitted when the writer is a terminal (isTTY), so piped
// logs (docker logs → files, CI) stay clean ASCII.
type color string

const (
	cReset  color = "\033[0m"
	cDim    color = "\033[2m"
	cRed    color = "\033[31m"
	cYellow color = "\x1b[33m"
	cBlue   color = "\x1b[34m"
	cCyan   color = "\x1b[36m"
	cWhite  color = "\x1b[37m"
)

func (c color) wrap(s string) string { return string(c) + s + string(cReset) }

// LevelFromEnv parses CONWAY_LOG_LEVEL, defaulting to info. Unrecognised
// values fall back with the level noted — a typo must not silence the log.
func LevelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("CONWAY_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// FormatFromEnv parses CONWAY_LOG_FORMAT: "json" for machine output, anything
// else for the colorized console.
func FormatFromEnv() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("CONWAY_LOG_FORMAT")))
}

// New returns the app logger. format is "color" or "json"; color is upgraded
// to JSON automatically when the output is not a terminal, because ANSI codes
// in a collected file are noise (and in some parsers, breakage).
func New(w io.Writer, format string, level slog.Level) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	}
	// "color": colors always on — pick json for piped/machine output.
	return slog.New(newColorHandler(w, &slog.HandlerOptions{Level: level}))
}

// NewDefault builds the logger Conway boots with: colorized console unless
// CONWAY_LOG_FORMAT=json, level from CONWAY_LOG_LEVEL.
func NewDefault() *slog.Logger {
	w := os.Stderr
	format := FormatFromEnv()
	if format == "" && !isTTY(w) {
		// Auto-detect only when the operator said nothing: a collected log
		// (systemd, docker, make) loses nothing to color codes, so it gets
		// JSON. An explicit CONWAY_LOG_FORMAT=color is honored — opt-in
		// means opt-in.
		format = "json"
	}
	if format == "" {
		format = "color"
	}
	return New(w, format, LevelFromEnv())
}

// isTTY reports whether w is a character device (a terminal). `os.Stderr`
// implements Stat; the Fd is a char device only for real terminals.
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

// ReplaceLogLogger routes the stdlib logger (and anything using it) through
// slog, so a stray fmt-style log.Printf still lands in the same shape.
func ReplaceLogStandard() {
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(slog.Default().Handler(), slog.LevelError).Writer())
}

// colorHandler is a slog.Handler rendering one line per record:
//
//	12:00:00 INFO  schedule computed  plan=PLAx dur=1.2s placed=28
//
// It exists because stdlib's TextHandler has no colors, and the app's logs are
// read by humans in a terminal far more often than by machines.
type colorHandler struct {
	opts  slog.HandlerOptions
	attrs []slog.Attr
	group string
	w     io.Writer
}

func newColorHandler(w io.Writer, opts *slog.HandlerOptions) *colorHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &colorHandler{w: w, opts: *opts}
}

func (h *colorHandler) Enabled(_ context.Context, l slog.Level) bool {
	floor := slog.LevelInfo
	if h.opts.Level != nil {
		floor = h.opts.Level.Level()
	}
	return l >= floor
}

// appendAttr renders one attribute as key=value, colored dimly; an error
// attribute is red, since that is usually the thing being looked for.
func (h *colorHandler) appendAttr(buf *[]byte, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindString && strings.Contains(a.Value.String(), "\n") {
		// multi-line values break one-line-per-record
		s := strconv.Quote(a.Value.String())
		*buf = append(*buf, (" " + a.Key + "=" + s)...)
		return
	}
	switch {
	case a.Value.Kind() == slog.KindAny && isErrVal(a.Value):
		*buf = append(*buf, (" " + a.Key + "=" + cRed.wrap(a.Value.String()))...)
	default:
		*buf = append(*buf, (" " + a.Key + "=" + cDim.wrap(a.Value.String()))...)
	}
}

func isErrVal(v slog.Value) bool {
	_, ok := v.Any().(error)
	return ok
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	buf := make([]byte, 0, 200)
	// Time: HH:MM:SS, dimmed. The date is noise in a tail.
	if !r.Time.IsZero() {
		buf = append(buf, r.Time.Format("15:04:05")...)
		buf = append(buf, ' ')
	}
	// Level, colored and fixed-width.
	buf = append(buf, h.levelText(r.Level)...)
	buf = append(buf, ' ')
	// The message.
	buf = append(buf, r.Message...)
	// Handler-level attrs, then record-level.
	for _, a := range h.attrs {
		h.appendAttr(&buf, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&buf, a)
		return true
	})
	buf = append(buf, '\n')
	_, err := h.w.Write(buf)
	return err
}

func (h *colorHandler) levelText(l slog.Level) string {
	s := l.String() // DEBUG/INFO/WARN/ERROR
	switch l {
	case slog.LevelError:
		return cRed.wrap(s)
	case slog.LevelWarn:
		return cYellow.wrap(s)
	case slog.LevelInfo:
		return cBlue.wrap(s)
	default:
		return cCyan.wrap(s)
	}
}

// WithAttrs and WithGroup clone the handler: slog's contract requires the
// receiver untouched.
func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &nh
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	nh.group = name
	return &nh
}

var _ slog.Handler = (*colorHandler)(nil)

// Escape sanitizes an untrusted string for a one-line log record: control
// characters and newlines become Go escapes, so a crafted value cannot forge
// additional log lines or escape the record's quoting. Use for any
// request-derived field (paths, emails, ids) passed into a log call.
func Escape(s string) string { return strconv.Quote(s) }
