package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// NewLogger returns the slog.Logger ctx uses for diagnostics: lines like
// "ctx: message key=value" on w, without timestamps, so CLI stderr output
// stays human-readable and stable across runs. The minimum level defaults
// to info and can be lowered/raised with CTX_LOG_LEVEL
// (debug|info|warn|error).
func NewLogger(w io.Writer) *slog.Logger {
	return slog.New(&logHandler{w: w, mu: &sync.Mutex{}, level: logLevelFromEnv()})
}

func logLevelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("CTX_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// logHandler renders records as "ctx: message key=value ..." with no
// timestamp. Groups are flattened into dotted attr names.
type logHandler struct {
	w      io.Writer
	mu     *sync.Mutex
	level  slog.Level
	attrs  []slog.Attr
	prefix string // dotted group prefix for attrs added after WithGroup
}

func (h *logHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString("ctx: ")
	sb.WriteString(r.Message)
	for _, attr := range h.attrs {
		// Stored attrs had their group prefix baked in by WithAttrs.
		writeAttr(&sb, "", attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		writeAttr(&sb, h.prefix, attr)
		return true
	})
	sb.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, sb.String())
	return err
}

func writeAttr(sb *strings.Builder, prefix string, attr slog.Attr) {
	if attr.Equal(slog.Attr{}) {
		return
	}
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		for _, sub := range value.Group() {
			writeAttr(sb, prefix+attr.Key+".", sub)
		}
		return
	}
	sb.WriteByte(' ')
	sb.WriteString(prefix)
	sb.WriteString(attr.Key)
	sb.WriteByte('=')
	text := value.String()
	if strings.ContainsAny(text, " \t\n\"") {
		text = fmt.Sprintf("%q", text)
	}
	sb.WriteString(text)
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append([]slog.Attr{}, h.attrs...)
	for _, attr := range attrs {
		attr.Key = h.prefix + attr.Key
		clone.attrs = append(clone.attrs, attr)
	}
	return &clone
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.prefix = h.prefix + name + "."
	return &clone
}
