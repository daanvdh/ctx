package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestGetRawAndAllowMissingAreMutuallyExclusive(t *testing.T) {
	err := Get(context.Background(), []string{"s1", "KEY", "--raw", "--allow-missing"})
	if err == nil {
		t.Fatal("expected mutual exclusivity error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Get error = %q, want mutual exclusivity message", err)
	}
}
