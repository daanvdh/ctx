package cmd

import "testing"

func TestSessionArgUsesExplicitSession(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := sessionArg([]string{"explicit"}, true)
	if err != nil {
		t.Fatalf("sessionArg error: %v", err)
	}
	if got != "explicit" {
		t.Fatalf("sessionArg = %q, want explicit", got)
	}
}

func TestSessionArgUsesEnvironmentWhenImplicit(t *testing.T) {
	t.Setenv("CTX_ID", "env")
	got, err := sessionArg([]string{"KEY", "VALUE"}, false)
	if err != nil {
		t.Fatalf("sessionArg error: %v", err)
	}
	if got != "env" {
		t.Fatalf("sessionArg = %q, want env", got)
	}
}

func TestSessionArgRequiresSession(t *testing.T) {
	t.Setenv("CTX_ID", "")
	if _, err := sessionArg(nil, false); err == nil {
		t.Fatal("expected missing session error")
	}
}
