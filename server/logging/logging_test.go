package logging

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestConsoleWriterShapesOneLinePerRecord(t *testing.T) {
	var b bytes.Buffer
	logger := New(&b, "console", zerolog.InfoLevel)
	logger.Info().Str("plan", "PLAx").Int("placed", 28).Msg("schedule computed")
	out := b.String()
	if !strings.Contains(out, "schedule computed") {
		t.Fatalf("message missing: %q", out)
	}
	if !strings.Contains(out, "PLAx") || !strings.Contains(out, "placed=") || !strings.Contains(out, "28") {
		t.Fatalf("attrs missing: %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("one line per record violated: %q", out)
	}
}

func TestJSONFormat(t *testing.T) {
	var b bytes.Buffer
	logger := New(&b, "json", zerolog.InfoLevel)
	logger.Warn().Str("user", "u1").Msg("rate limited")
	out := b.String()
	if !strings.Contains(out, `"level":"warn"`) || !strings.Contains(out, `"user":"u1"`) {
		t.Fatalf("json shape wrong: %q", out)
	}
}

func TestErrorChain(t *testing.T) {
	var b bytes.Buffer
	logger := New(&b, "json", zerolog.InfoLevel)
	logger.Error().Err(errors.New("boom")).Str("plan", "PLAx").Msg("import failed")
	out := b.String()
	if !strings.Contains(out, `"error":"boom"`) {
		t.Fatalf("error field missing: %q", out)
	}
}

func TestDebugSuppressedAtInfo(t *testing.T) {
	var b bytes.Buffer
	logger := New(&b, "json", zerolog.InfoLevel)
	logger.Debug().Int("steps", 42).Msg("engine internals")
	if b.Len() != 0 {
		t.Fatalf("debug leaked at info level: %q", b.String())
	}
}

func TestEscapeSanitizesNewlines(t *testing.T) {
	got := Escape("line1\nline2 forged ERROR record")
	if strings.Contains(got, "\n") {
		t.Fatalf("newline survived: %q", got)
	}
	if !strings.Contains(got, "\\n") {
		t.Fatalf("expected escaped newline: %q", got)
	}
}

func TestLevelFromEnv(t *testing.T) {
	t.Setenv("CONWAY_LOG_LEVEL", "debug")
	if LevelFromEnv() != zerolog.DebugLevel {
		t.Fatal("debug not parsed")
	}
	t.Setenv("CONWAY_LOG_LEVEL", "nonsense")
	if LevelFromEnv() != zerolog.InfoLevel {
		t.Fatal("unrecognised level must fall back to info")
	}
}
