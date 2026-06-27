package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ctx/internal/model"
)

func TestTreeSingleRoot(t *testing.T) {
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"abc123": {
				Parent: nil,
				Data: map[string]string{
					"PROJECT_ID": "gitlab-org/myproject",
					"MR_IID":     "412",
				},
			},
		},
	}

	output, err := Tree(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "abc123") {
		t.Errorf("output should contain session ID abc123, got: %s", output)
	}
	if !strings.Contains(output, "PROJECT_ID [string] gitlab-org/myproject") {
		t.Errorf("output should contain PROJECT_ID, got: %s", output)
	}
	if !strings.Contains(output, "MR_IID [string] 412") {
		t.Errorf("output should contain MR_IID, got: %s", output)
	}
}

func TestTreeRootWithOneChild(t *testing.T) {
	parent := "abc123"
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"abc123": {
				Parent: nil,
				Data: map[string]string{
					"PROJECT_ID": "gitlab-org/myproject",
					"MR_IID":     "412",
				},
			},
			"def456": {
				Parent: &parent,
				Data: map[string]string{
					"DISCUSSION_ID": "abc123def456",
				},
			},
		},
	}

	output, err := Tree(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "def456") {
		t.Errorf("output should contain child session ID def456, got: %s", output)
	}
	if !strings.Contains(output, "└── def456") {
		t.Errorf("output should contain └── connector for single child, got: %s", output)
	}
	if !strings.Contains(output, "DISCUSSION_ID [string] abc123def456") {
		t.Errorf("output should contain child data, got: %s", output)
	}
}

func TestTreeRootWithTwoChildren(t *testing.T) {
	parent := "abc123"
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"abc123": {
				Parent: nil,
				Data: map[string]string{
					"PROJECT_ID": "gitlab-org/myproject",
				},
			},
			"def456": {
				Parent: &parent,
				Data: map[string]string{
					"DISCUSSION_ID": "abc123def456",
				},
			},
			"ghi789": {
				Parent: &parent,
				Data: map[string]string{
					"DISCUSSION_ID": "xyz789abc012",
				},
			},
		},
	}

	output, err := Tree(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "├── def456") {
		t.Errorf("output should contain ├── connector for first child, got: %s", output)
	}
	if !strings.Contains(output, "└── ghi789") {
		t.Errorf("output should contain └── connector for last child, got: %s", output)
	}
	if !strings.Contains(output, "DISCUSSION_ID [string] abc123def456") {
		t.Errorf("output should contain first child data, got: %s", output)
	}
	if !strings.Contains(output, "DISCUSSION_ID [string] xyz789abc012") {
		t.Errorf("output should contain last child data, got: %s", output)
	}
}

func TestTreeMultiLevel(t *testing.T) {
	grandparent := "abc123"
	parent := "def456"
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"abc123": {
				Parent: nil,
				Data: map[string]string{
					"PROJECT_ID": "gitlab-org/myproject",
					"MR_IID":     "412",
				},
			},
			"def456": {
				Parent: &grandparent,
				Data: map[string]string{
					"DISCUSSION_ID": "abc123def456",
				},
			},
			"jkl012": {
				Parent: &parent,
				Data: map[string]string{
					"CHUNK_ID": "zzz",
				},
			},
		},
	}

	output, err := Tree(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "abc123") {
		t.Errorf("output should contain root abc123, got: %s", output)
	}
	if !strings.Contains(output, "def456") {
		t.Errorf("output should contain child def456, got: %s", output)
	}
	if !strings.Contains(output, "jkl012") {
		t.Errorf("output should contain grandchild jkl012, got: %s", output)
	}
	if !strings.Contains(output, "CHUNK_ID [string] zzz") {
		t.Errorf("output should contain grandchild data, got: %s", output)
	}
}

func TestTreeNilInput(t *testing.T) {
	output, err := Tree(nil)
	if err != nil {
		t.Fatalf("unexpected error on nil input: %v", err)
	}
	if output != "" {
		t.Errorf("expected empty output for nil input, got: %s", output)
	}
}

func TestTreeNilSessions(t *testing.T) {
	cf := &model.ContextFile{
		Sessions: nil,
	}

	output, err := Tree(cf)
	if err != nil {
		t.Fatalf("unexpected error on nil sessions: %v", err)
	}
	if output != "" {
		t.Errorf("expected empty output for nil sessions, got: %s", output)
	}
}

func TestTreeFormatsFileRefAsPath(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "spec.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.0.0\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"root": {
				Data: map[string]string{"SPEC": path},
				Entries: map[string]model.Entry{
					"SPEC": model.NewEntry(path, model.ValueTypeFileRef),
				},
			},
		},
	}

	output, err := Tree(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "SPEC [path] "+path) {
		t.Fatalf("output = %s, want [path] label", output)
	}
}

func TestTreeDeterministicOutput(t *testing.T) {
	parent := "root_session"
	parent2 := parent

	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"root_session": {
				Parent: nil,
				Data: map[string]string{
					"KEY": "root",
				},
			},
			"session_c": {
				Parent: &parent2,
				Data: map[string]string{
					"KEY": "c",
				},
			},
			"session_a": {
				Parent: &parent2,
				Data: map[string]string{
					"KEY": "a",
				},
			},
			"session_b": {
				Parent: &parent2,
				Data: map[string]string{
					"KEY": "b",
				},
			},
		},
	}

	output, err := Tree(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sessions should be alphabetically sorted
	firstChildIdx := strings.Index(output, "session_a")
	secondChildIdx := strings.Index(output, "session_b")
	thirdChildIdx := strings.Index(output, "session_c")

	if firstChildIdx >= 0 && secondChildIdx >= 0 && firstChildIdx >= secondChildIdx {
		t.Errorf("output should have session_a before session_b, got a at %d, b at %d", firstChildIdx, secondChildIdx)
	}
	if secondChildIdx >= 0 && thirdChildIdx >= 0 && secondChildIdx >= thirdChildIdx {
		t.Errorf("output should have session_b before session_c, got b at %d, c at %d", secondChildIdx, thirdChildIdx)
	}
}
