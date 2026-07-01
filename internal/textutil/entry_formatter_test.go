package textutil

import (
	"strings"
	"testing"

	"ctx/internal/model"
)

func TestPreviewUsesFirstLineOnly(t *testing.T) {
	got := Preview("first line\nsecond line", 60)
	if got != "first line ..." {
		t.Fatalf("Preview = %q, want first line with truncation marker", got)
	}
}

func TestPreviewSingleLineUnderMaxIsUnchanged(t *testing.T) {
	got := Preview("short value", 60)
	if got != "short value" {
		t.Fatalf("Preview = %q, want value unchanged", got)
	}
}

func TestPreviewTruncatesFirstLine(t *testing.T) {
	got := Preview("abcdefghij\nsecond", 8)
	if got != "abcd ..." {
		t.Fatalf("Preview = %q, want truncated first line", got)
	}
}

func TestPreviewTrailingNewlineWithNoContentAfter(t *testing.T) {
	got := Preview("only line\n", 60)
	if got != "only line" {
		t.Fatalf("Preview = %q, want no truncation marker for trailing empty line", got)
	}
}

func TestLineFormatsDocWithoutEscapedNewlines(t *testing.T) {
	got := Line("DOC", model.NewEntry("first line\nsecond line", model.ValueTypeDoc))
	if strings.Contains(got, `\n`) || strings.Contains(got, "second line") {
		t.Fatalf("Line = %q, want first-line preview without escaped newlines", got)
	}
	if !strings.Contains(got, "DOC [doc]") || !strings.Contains(got, "first line") {
		t.Fatalf("Line = %q, want doc label and preview", got)
	}
}

func TestLineFormatsStringWithPreview(t *testing.T) {
	got := Line("TEXT", model.NewEntry("first line\nsecond line", model.ValueTypeString))
	if got != "TEXT [string] first line ..." {
		t.Fatalf("Line = %q, want first-line string preview with truncation marker", got)
	}
}
