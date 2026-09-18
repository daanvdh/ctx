package trigger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRejectsInvalidYAML(t *testing.T) {
	_, err := Parse("test.md", "script: [unclosed\n")
	if err == nil || !strings.Contains(err.Error(), "malformed trigger test.md") {
		t.Fatalf("Parse error = %v, want malformed trigger error", err)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse("test.md", "script: echo\nscritp-typo: oops\n")
	if err == nil || !strings.Contains(err.Error(), "malformed trigger test.md") {
		t.Fatalf("Parse error = %v, want unknown-field error", err)
	}
}

func TestParseRejectsWrongFieldType(t *testing.T) {
	_, err := Parse("test.md", "script: echo\nlogging: sometimes\n")
	if err == nil || !strings.Contains(err.Error(), "malformed trigger test.md") {
		t.Fatalf("Parse error = %v, want type error", err)
	}
}

func TestParseRejectsEmptyContent(t *testing.T) {
	_, err := Parse("test.md", "")
	if err == nil || !strings.Contains(err.Error(), "missing script") {
		t.Fatalf("Parse error = %v, want missing script error", err)
	}
}

func TestParseRejectsEntriesWithoutValueKey(t *testing.T) {
	_, err := Parse("test.md", "script: echo\nentries:\n  STATUS:\n    - DONE\n")
	if err == nil || !strings.Contains(err.Error(), "malformed trigger test.md") {
		t.Fatalf("Parse error = %v, want malformed entries error (values must be '- value: X' mappings)", err)
	}
}

func TestParseEmptyEntriesListIsWildcard(t *testing.T) {
	def, err := Parse("test.md", "script: echo\nentries:\n  STATUS: []\n")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	values, ok := def.Entries["STATUS"]
	if !ok || len(values) != 0 {
		t.Fatalf("Entries = %#v, want STATUS as empty wildcard", def.Entries)
	}
}

func TestParseFrontmatterOnlyBodySeparator(t *testing.T) {
	def, err := Parse("test.md", "script: echo\n---\nprompt body $VAR\n")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if def.PromptTemplate != "prompt body $VAR\n" {
		t.Fatalf("PromptTemplate = %q, want body after separator", def.PromptTemplate)
	}
}

func TestLoadAllPropagatesParseErrorWithPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}
	bad := filepath.Join(triggerDir, "bad.md")
	if err := os.WriteFile(bad, []byte("no-script: true\n"), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	_, err := LoadAll()
	if err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatalf("LoadAll error = %v, want parse error naming %s", err, bad)
	}
}

func TestLoadAllMissingDirectoryYieldsNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	defs, err := LoadAll()
	if err != nil || len(defs) != 0 {
		t.Fatalf("LoadAll = %v, %v, want empty and no error", defs, err)
	}
}

func TestLoadAllSkipsDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(filepath.Join(triggerDir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}
	if err := os.WriteFile(filepath.Join(triggerDir, "ok.md"), []byte("script: echo\n"), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	defs, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "ok" {
		t.Fatalf("LoadAll = %#v, want just the ok trigger", defs)
	}
}
