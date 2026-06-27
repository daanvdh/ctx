// Package textutil centralizes human-readable formatting for typed ctx entries.
package textutil

import (
	"fmt"
	"os"
	"strings"

	"ctx/internal/model"
)

// PreviewChars is the default maximum number of characters shown in entry previews.
const PreviewChars = 60

// Line returns the single-line display form for a typed entry.
func Line(key string, entry model.Entry) string {
	switch entry.ValueType {
	case model.ValueTypeDoc:
		return fmt.Sprintf("%s [doc] %s %s", key, HumanSize(len([]byte(entry.Value))), Preview(entry.Value, PreviewChars))
	case model.ValueTypeFileRef:
		if _, err := os.Stat(entry.Value); err != nil && os.IsNotExist(err) {
			return fmt.Sprintf("%s [path] %s", key, "\u26a0 path not found")
		}
		return fmt.Sprintf("%s [path] %s", key, entry.Value)
	case model.ValueTypeString:
		return fmt.Sprintf("%s [string] %s", key, Preview(entry.Value, PreviewChars))
	default:
		return fmt.Sprintf("%s [%s] not implemented", key, entry.ValueType)
	}
}

// Preview returns the first line of a value, truncated to max characters.
func Preview(value string, max int) string {
	if max <= 0 {
		return ""
	}
	line, _, _ := strings.Cut(value, "\n")
	line = strings.TrimSuffix(line, "\r")
	runes := []rune(line)
	if len(runes) <= max {
		return line
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

// HumanSize formats a byte count for compact display.
func HumanSize(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024.0)
}
