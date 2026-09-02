package logging

import (
	"bytes"
	"errors"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
)

var _ = Describe("logging", func() {
	Describe("the console writer", func() {
		It("shapes one line per record with message and attrs", func() {
			var b bytes.Buffer
			logger := New(&b, "console", zerolog.InfoLevel)
			logger.Info().Str("plan", "PLAx").Int("placed", 28).Msg("schedule computed")
			out := b.String()
			Expect(out).To(ContainSubstring("schedule computed"))
			Expect(out).To(ContainSubstring("PLAx"))
			Expect(out).To(ContainSubstring("placed="))
			Expect(out).To(ContainSubstring("28"))
			Expect(strings.Count(out, "\n")).To(Equal(1), "one line per record")
		})

		It("escapes newlines in field values, so a forged record cannot inject lines", func() {
			var b bytes.Buffer
			logger := New(&b, "console", zerolog.InfoLevel)
			logger.Info().Str("path", "/api/plan/x\nFORGED-ENTRY password=leaked").Msg("http request")
			out := b.String()
			Expect(out).To(ContainSubstring("\\n"), "the newline is escaped in the rendered value")
			Expect(out).NotTo(ContainSubstring("FORGED-ENTRY password=leaked\n"), "no raw newline for the forged text")
			Expect(strings.Count(out, "\n")).To(Equal(1))
		})
	})

	Describe("the JSON format", func() {
		It("emits one machine-readable object per record", func() {
			var b bytes.Buffer
			logger := New(&b, "json", zerolog.InfoLevel)
			logger.Warn().Str("user", "u1").Msg("rate limited")
			out := b.String()
			Expect(out).To(ContainSubstring(`"level":"warn"`))
			Expect(out).To(ContainSubstring(`"user":"u1"`))
		})

		It("carries the error field for .Err chains", func() {
			var b bytes.Buffer
			logger := New(&b, "json", zerolog.InfoLevel)
			logger.Error().Err(errors.New("boom")).Str("plan", "PLAx").Msg("import failed")
			Expect(b.String()).To(ContainSubstring(`"error":"boom"`))
		})

		It("suppresses debug at the info level", func() {
			var b bytes.Buffer
			logger := New(&b, "json", zerolog.InfoLevel)
			logger.Debug().Int("steps", 42).Msg("engine internals")
			Expect(b.Len()).To(BeZero(), "debug leaked at info level")
		})
	})

	Describe("Escape", func() {
		It("sanitizes newlines for message interpolation", func() {
			got := Escape("line1\nline2 forged ERROR record")
			Expect(got).NotTo(ContainSubstring("\n"))
			Expect(got).To(ContainSubstring("\\n"))
		})
	})

	Describe("LevelFromEnv", func() {
		It("parses the standard levels and falls back to info on nonsense", func() {
			os.Setenv("CONWAY_LOG_LEVEL", "debug")
			DeferCleanup(os.Unsetenv, "CONWAY_LOG_LEVEL")
			Expect(LevelFromEnv()).To(Equal(zerolog.DebugLevel))
			os.Setenv("CONWAY_LOG_LEVEL", "nonsense")
			Expect(LevelFromEnv()).To(Equal(zerolog.InfoLevel), "a typo must not silence the log")
		})
	})
})
