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

func TestTemplateStringAllowMissing(t *testing.T) {
	got, err := TemplateStringWithOptions("known $A missing $B", map[string]string{"A": "1"}, TemplateOptions{AllowMissing: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "known 1 missing $B" {
		t.Fatalf("got %q, want missing placeholder preserved", got)
	}
}

func TestTemplateStringRecursiveExpandsNestedPlaceholders(t *testing.T) {
	resolved := map[string]string{"A": "$B", "B": "$C", "C": "done"}
	got, err := TemplateStringRecursive("start $A", resolved, TemplateOptions{}, MaxRenderDepth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "start done" {
		t.Fatalf("got %q, want fully expanded nested placeholders", got)
	}
}

func TestTemplateStringRecursiveStopsAtMaxDepthOnCycle(t *testing.T) {
	resolved := map[string]string{"A": "$B", "B": "$A"}
	got, err := TemplateStringRecursive("$A", resolved, TemplateOptions{}, MaxRenderDepth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "$A" && got != "$B" {
		t.Fatalf("got %q, want recursion to stop after MaxRenderDepth passes instead of looping forever", got)
	}
}

func TestExtractVarNamesOrderAndDedup(t *testing.T) {
	got := ExtractVarNames(`echo "$TITLE" $DESCRIPTION; echo again: "$TITLE"`)
	want := []string{"TITLE", "DESCRIPTION"}
	if len(got) != len(want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names = %v, want %v", got, want)
		}
	}
}

func TestExtractVarNamesSkipsEscaped(t *testing.T) {
	got := ExtractVarNames("literal $$ISSUE and real $STORY")
	if len(got) != 1 || got[0] != "STORY" {
		t.Fatalf("names = %v, want [STORY]", got)
	}
}

func TestRewriteVarsReplacesKnownNames(t *testing.T) {
	got := RewriteVars(`echo "$TITLE" $STORY_ID`, map[string]int{"TITLE": 1})
	want := `echo "$1" $STORY_ID`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteVarsPreservesEscape(t *testing.T) {
	got := RewriteVars("literal $$ISSUE and real $ISSUE", map[string]int{"ISSUE": 1})
	want := "literal $ISSUE and real $1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
