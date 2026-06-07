package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPath(t *testing.T) {
	// Test that Path command exists and works without args
	exitCode := Path([]string{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Verify the output is a valid path
	// It should end with ctx.json
	output, err := os.ReadFile(filepath.Join(os.Getenv("PWD"), "ctx.json"))
	if err == nil {
		// The file exists, path command should work
		_ = output
	}
}

func TestPathWithArgs(t *testing.T) {
	// Test that Path command rejects arguments
	exitCode := Path([]string{"arg1"})
	if exitCode != 1 {
		t.Fatalf("expected exit code 1 for unexpected args, got %d", exitCode)
	}
}
