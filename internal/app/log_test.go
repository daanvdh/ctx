package app

import (
	"strings"
	"testing"
)

func TestLoggerFormatsCtxPrefixedLines(t *testing.T) {
	var buf strings.Builder
	logger := NewLogger(&buf)

	logger.Warn("trigger depth limit reached", "limit", 5, "key", "STATUS")
	if got, want := buf.String(), "ctx: trigger depth limit reached limit=5 key=STATUS\n"; got != want {
		t.Fatalf("log line = %q, want %q", got, want)
	}
}

func TestLoggerQuotesValuesWithSpaces(t *testing.T) {
	var buf strings.Builder
	logger := NewLogger(&buf)

	logger.Error("trigger failed", "error", "exit status 1")
	if got, want := buf.String(), "ctx: trigger failed error=\"exit status 1\"\n"; got != want {
		t.Fatalf("log line = %q, want %q", got, want)
	}
}

func TestLoggerLevelFromEnv(t *testing.T) {
	t.Setenv("CTX_LOG_LEVEL", "error")
	var buf strings.Builder
	logger := NewLogger(&buf)

	logger.Warn("should be suppressed")
	logger.Error("should appear")
	if got, want := buf.String(), "ctx: should appear\n"; got != want {
		t.Fatalf("log output = %q, want only the error line %q", got, want)
	}
}

func TestLoggerWithAttrsAndGroups(t *testing.T) {
	var buf strings.Builder
	logger := NewLogger(&buf).With("source", "github").WithGroup("delivery")

	logger.Info("webhook stored", "id", "abc")
	if got, want := buf.String(), "ctx: webhook stored source=github delivery.id=abc\n"; got != want {
		t.Fatalf("log line = %q, want %q", got, want)
	}
}
