package cmd

import "testing"

func TestParseSessionArgsNoArgs(t *testing.T) {
	got, err := parseSessionArgs(nil)
	if err != nil {
		t.Fatalf("parseSessionArgs error: %v", err)
	}
	if got.name != "" || got.parent != nil || got.root {
		t.Fatalf("parseSessionArgs = %#v, want all defaults", got)
	}
}

func TestParseSessionArgsName(t *testing.T) {
	got, err := parseSessionArgs([]string{"myname"})
	if err != nil {
		t.Fatalf("parseSessionArgs error: %v", err)
	}
	if got.name != "myname" || got.parent != nil || got.root {
		t.Fatalf("parseSessionArgs = %#v, want name only", got)
	}
}

func TestParseSessionArgsParentAndName(t *testing.T) {
	got, err := parseSessionArgs([]string{"parent1", "myname"})
	if err != nil {
		t.Fatalf("parseSessionArgs error: %v", err)
	}
	if got.name != "myname" || got.parent == nil || *got.parent != "parent1" {
		t.Fatalf("parseSessionArgs = %#v, want parent1/myname", got)
	}
}

func TestParseSessionArgsParentFlag(t *testing.T) {
	got, err := parseSessionArgs([]string{"--parent", "p"})
	if err != nil {
		t.Fatalf("parseSessionArgs error: %v", err)
	}
	if got.name != "" || got.parent == nil || *got.parent != "p" {
		t.Fatalf("parseSessionArgs = %#v, want parent flag only", got)
	}
}

func TestParseSessionArgsParentFlagAndName(t *testing.T) {
	got, err := parseSessionArgs([]string{"--parent", "p", "myname"})
	if err != nil {
		t.Fatalf("parseSessionArgs error: %v", err)
	}
	if got.name != "myname" || got.parent == nil || *got.parent != "p" {
		t.Fatalf("parseSessionArgs = %#v, want parent flag and name", got)
	}
}

func TestParseSessionArgsRoot(t *testing.T) {
	got, err := parseSessionArgs([]string{"--root"})
	if err != nil {
		t.Fatalf("parseSessionArgs error: %v", err)
	}
	if got.name != "" || got.parent != nil || !got.root {
		t.Fatalf("parseSessionArgs = %#v, want root only", got)
	}
}

func TestParseSessionArgsRootAndName(t *testing.T) {
	got, err := parseSessionArgs([]string{"--root", "myname"})
	if err != nil {
		t.Fatalf("parseSessionArgs error: %v", err)
	}
	if got.name != "myname" || got.parent != nil || !got.root {
		t.Fatalf("parseSessionArgs = %#v, want root and name", got)
	}
}

func TestParseSessionArgsParentAndRootConflict(t *testing.T) {
	if _, err := parseSessionArgs([]string{"--parent", "p", "--root"}); err == nil {
		t.Fatal("expected error for --parent and --root together")
	}
}

func TestParseSessionArgsPositionalAndFlagParentConflict(t *testing.T) {
	if _, err := parseSessionArgs([]string{"parent1", "myname", "--parent", "p"}); err == nil {
		t.Fatal("expected error for positional parent and --parent together")
	}
}

func TestParseSessionArgsPositionalParentAndRootConflict(t *testing.T) {
	if _, err := parseSessionArgs([]string{"parent1", "myname", "--root"}); err == nil {
		t.Fatal("expected error for positional parent and --root together")
	}
}

func TestParseSessionArgsTooManyPositional(t *testing.T) {
	if _, err := parseSessionArgs([]string{"a", "b", "c"}); err == nil {
		t.Fatal("expected error for too many positional arguments")
	}
}

func TestParseSessionArgsMissingParentValue(t *testing.T) {
	if _, err := parseSessionArgs([]string{"--parent"}); err == nil {
		t.Fatal("expected error for missing --parent value")
	}
}

func TestParseSessionRmArgsSession(t *testing.T) {
	target, recursive, err := parseSessionRmArgs([]string{"mysession"})
	if err != nil {
		t.Fatalf("parseSessionRmArgs error: %v", err)
	}
	if target != "mysession" || recursive {
		t.Fatalf("parseSessionRmArgs = %q, %v, want mysession, false", target, recursive)
	}
}

func TestParseSessionRmArgsRecursive(t *testing.T) {
	target, recursive, err := parseSessionRmArgs([]string{"mysession", "--recursive"})
	if err != nil {
		t.Fatalf("parseSessionRmArgs error: %v", err)
	}
	if target != "mysession" || !recursive {
		t.Fatalf("parseSessionRmArgs = %q, %v, want mysession, true", target, recursive)
	}
}

func TestParseSessionRmArgsTooManyPositional(t *testing.T) {
	if _, _, err := parseSessionRmArgs([]string{"a", "b"}); err == nil {
		t.Fatal("expected error for too many positional arguments")
	}
}

func TestParseSessionRmArgsMissingSession(t *testing.T) {
	t.Setenv("CTX_ID", "")
	if _, _, err := parseSessionRmArgs(nil); err == nil {
		t.Fatal("expected error when no session given and CTX_ID unset")
	}
}
