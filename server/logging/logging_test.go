package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestColorHandlerShapesOneLinePerRecord(t *testing.T) {
	var b bytes.Buffer
	h := newColorHandler(&b, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(h)
	logger.Info("schedule computed", "plan", "PLAx", "placed", 28)
	out := b.String()
	if !strings.Contains(out, "schedule computed") {
		t.Fatalf("message missing: %q", out)
	}
	if !strings.Contains(out, "plan=") || !strings.Contains(out, "PLAx") || !strings.Contains(out, "placed=") || !strings.Contains(out, "28") {
		t.Fatalf("attrs missing: %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("one line per record violated: %q", out)
	}
}

func TestColorHandlerLevelsAndError(t *testing.T) {
	var b bytes.Buffer
	logger := slog.New(newColorHandler(&b, nil))
	logger.Error("import failed", "err", errors.New("boom"), "plan", "PLAx")
	out := b.String()
	if !strings.Contains(out, "ERROR") {
		t.Fatalf("level missing: %q", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected colors: the color handler always colors; use New(.., \"json\", ..) for machines: %q", out)
	}
	// the error value is rendered, not dropped
	if !strings.Contains(out, "boom") {
		t.Fatalf("error text missing: %q", out)
	}
}

func TestDebugSuppressedAtInfo(t *testing.T) {
	var b bytes.Buffer
	logger := slog.New(newColorHandler(&b, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Debug("engine internals", "steps", 42)
	if b.Len() != 0 {
		t.Fatalf("debug leaked at info level: %q", b.String())
	}
}

func TestWithAttrsCarriesThrough(t *testing.T) {
	var b bytes.Buffer
	logger := slog.New(newColorHandler(&b, nil)).With("req", "abc")
	logger.Info("handled")
	if !strings.Contains(b.String(), "req=") || !strings.Contains(b.String(), "abc") {
		t.Fatalf("handler attrs missing: %q", b.String())
	}
}

func TestJSONFormat(t *testing.T) {
	var b bytes.Buffer
	logger := New(&b, "json", slog.LevelInfo)
	logger.Warn("rate limited", "user", "u1")
	out := b.String()
	if !strings.Contains(out, `"level":"WARN"`) || !strings.Contains(out, `"user":"u1"`) {
		t.Fatalf("json shape wrong: %q", out)
	}
}
