package render

import (
	"ctx/internal/model"
	"strings"
	"testing"
)

func TestRenderPlaceholder(t *testing.T) {
	parentID := "root"
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"root": {
				Parent: nil,
				Data:   map[string]string{"ISSUE": "22"},
			},
			"s1": {
				Parent: &parentID,
				Data:   map[string]string{"STORY_PROMPT": "Fix issue $ISSUE"},
			},
		},
	}

	result, err := Render(cf, "s1", "STORY_PROMPT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Fix issue 22"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestRenderMissingPlaceholder(t *testing.T) {
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"s1": {
				Parent: nil,
				Data:   map[string]string{"STORY_PROMPT": "Fix issue $ISSUE"},
			},
		},
	}

	_, err := Render(cf, "s1", "STORY_PROMPT")
	if err == nil {
		t.Fatalf("expected error for missing placeholder, got nil")
	}
}

func TestTemplateStringEscapedPlaceholder(t *testing.T) {
	got, err := TemplateString("literal $$ISSUE and real $ISSUE", map[string]string{"ISSUE": "22"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "literal $ISSUE and real 22"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTemplateStringMissingPlaceholdersDeterministic(t *testing.T) {
	_, err := TemplateString("$B $A $B", nil)
	if err == nil {
		t.Fatal("expected missing placeholder error")
	}
	if !strings.Contains(err.Error(), "[A B]") {
		t.Fatalf("error = %q, want sorted unique placeholders [A B]", err.Error())
	}
}
