package cmd

import "testing"

func TestParseRmArgsWithExplicitSession(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := parseRmArgs([]string{"s1", "KEY"})
	if err != nil {
		t.Fatalf("parseRmArgs error: %v", err)
	}
	if got.sessionID != "s1" || got.entry != "KEY" {
		t.Fatalf("parseRmArgs = %#v, want explicit session and entry", got)
	}
}

func TestParseRmArgsUsesEnvironmentSession(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := parseRmArgs([]string{"KEY"})
	if err != nil {
		t.Fatalf("parseRmArgs error: %v", err)
	}
	if got.sessionID != "env" || got.entry != "KEY" {
		t.Fatalf("parseRmArgs = %#v, want environment session and entry", got)
	}
}

func TestParseRmArgsRequiresSession(t *testing.T) {
	t.Setenv("CTX_ID", "")
	if _, err := parseRmArgs([]string{"KEY"}); err == nil {
		t.Fatal("expected missing session error")
	}
}
