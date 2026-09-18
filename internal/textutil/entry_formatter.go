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
	case model.ValueTypeFileRef:
		if _, err := os.Stat(entry.Value); err != nil && os.IsNotExist(err) {
			return fmt.Sprintf("%s: [path] %s", key, "\u26a0 path not found")
		}
		return fmt.Sprintf("%s: [path] %s", key, entry.Value)
	case model.ValueTypeString:
		return fmt.Sprintf("%s: %s", key, Preview(entry.Value, PreviewChars))
	default:
		return fmt.Sprintf("%s [%s] not implemented", key, entry.ValueType)
	}
}

// FullLine returns the display line for a typed entry using already-resolved
// content, bypassing the per-type preview/truncation used by Line. Only
// file_ref entries show their type label; other types print like a plain
// string.
func FullLine(key string, entry model.Entry, content string) string {
	if entry.ValueType == model.ValueTypeFileRef {
		return fmt.Sprintf("%s: [path] %s", key, content)
	}
	return fmt.Sprintf("%s: %s", key, content)
}

// Preview returns the first line of a value, truncated to max characters.
// Whenever content is cut off, whether because the first line exceeds max
// characters or because the value has additional lines, " ..." is appended.
func Preview(value string, max int) string {
	if max <= 0 {
		return ""
	}
	line, rest, hadNewline := strings.Cut(value, "\n")
	line = strings.TrimSuffix(line, "\r")
	runes := []rune(line)

	lineTruncated := len(runes) > max
	moreLines := hadNewline && rest != ""

	if !lineTruncated {
		if moreLines {
			return line + " ..."
		}
		return line
	}
	if max <= 4 {
		return string(runes[:max])
	}
	return string(runes[:max-4]) + " ..."
}
