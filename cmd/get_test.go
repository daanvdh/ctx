package cmd

import "testing"

func TestParseGetArgsAllowsRawAndAllowMissingTogether(t *testing.T) {
	got, err := parseGetArgs([]string{"s1", "KEY", "--raw", "--allow-missing"})
	if err != nil {
		t.Fatalf("parseGetArgs error: %v", err)
	}
	if got.sessionID != "s1" || got.key != "KEY" {
		t.Fatalf("parseGetArgs = %#v, want session s1 and key KEY", got)
	}
	if !got.raw || !got.allowMissing {
		t.Fatalf("parseGetArgs = %#v, want raw and allowMissing both set", got)
	}
}

func TestParseGetArgsUsesEnvironmentSession(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := parseGetArgs([]string{"KEY"})
	if err != nil {
		t.Fatalf("parseGetArgs error: %v", err)
	}
	if got.sessionID != "env" || got.key != "KEY" {
		t.Fatalf("parseGetArgs = %#v, want environment session and key", got)
	}
}
