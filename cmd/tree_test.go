package cmd

import "testing"

func TestParseTreeArgsUsesEnvironmentSession(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := parseTreeArgs(nil)
	if err != nil {
		t.Fatalf("parseTreeArgs error: %v", err)
	}
	if got.sessionID != "env" {
		t.Fatalf("parseTreeArgs = %#v, want environment session", got)
	}
}

func TestParseTreeArgsWithExplicitSession(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := parseTreeArgs([]string{"s1"})
	if err != nil {
		t.Fatalf("parseTreeArgs error: %v", err)
	}
	if got.sessionID != "s1" {
		t.Fatalf("parseTreeArgs = %#v, want explicit session", got)
	}
}

func TestParseTreeArgsNoSessionNoEnvShowsEverything(t *testing.T) {
	t.Setenv("CTX_ID", "")
	got, err := parseTreeArgs(nil)
	if err != nil {
		t.Fatalf("parseTreeArgs error: %v", err)
	}
	if got.sessionID != "" {
		t.Fatalf("parseTreeArgs = %#v, want empty session id", got)
	}
}

func TestParseTreeArgsAllFlagOverridesSession(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := parseTreeArgs([]string{"s1", "-a"})
	if err != nil {
		t.Fatalf("parseTreeArgs error: %v", err)
	}
	if got.sessionID != "" {
		t.Fatalf("parseTreeArgs = %#v, want empty session id with -a", got)
	}
}

func TestParseTreeArgsAllFlagWithoutSessionOrEnv(t *testing.T) {
	t.Setenv("CTX_ID", "")
	got, err := parseTreeArgs([]string{"--all"})
	if err != nil {
		t.Fatalf("parseTreeArgs error: %v", err)
	}
	if got.sessionID != "" {
		t.Fatalf("parseTreeArgs = %#v, want empty session id", got)
	}
}

func TestParseTreeArgsFormatFlags(t *testing.T) {
	t.Setenv("CTX_ID", "")
	got, err := parseTreeArgs([]string{"--json"})
	if err != nil {
		t.Fatalf("parseTreeArgs error: %v", err)
	}
	if got.format != "json" {
		t.Fatalf("format = %q, want json", got.format)
	}

	got, err = parseTreeArgs([]string{"--format", "JSON"})
	if err != nil {
		t.Fatalf("parseTreeArgs error: %v", err)
	}
	if got.format != "json" {
		t.Fatalf("format = %q, want json", got.format)
	}
}

func TestParseTreeArgsRejectsMultipleSessionIDs(t *testing.T) {
	if _, err := parseTreeArgs([]string{"s1", "s2"}); err == nil {
		t.Fatal("expected usage error for multiple session ids")
	}
}

func TestParseTreeArgsRejectsUnknownFlag(t *testing.T) {
	if _, err := parseTreeArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected usage error for unknown flag")
	}
}
