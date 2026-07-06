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
