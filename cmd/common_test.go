package cmd

import "testing"

func TestHelpRequested(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"KEY", "VALUE"}, false},
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{[]string{"session", "-h"}, true},
	}
	for _, c := range cases {
		if got := helpRequested(c.args); got != c.want {
			t.Fatalf("helpRequested(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

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
