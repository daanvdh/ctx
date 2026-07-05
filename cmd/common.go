package cmd

import (
	"context"
	"fmt"
	"os"

	"ctx/internal/app"
)

type Command struct {
	Name string
	Run  func(context.Context, []string) error
}

func newApp() (*app.App, error) {
	return app.New()
}

func usage(_ string, text string) error {
	return fmt.Errorf("usage: %s", text)
}

// helpRequested reports whether args contains --help or -h.
func helpRequested(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

func sessionArg(args []string, hasExplicitSession bool) (string, error) {
	if hasExplicitSession {
		return args[0], nil
	}
	sessionID := os.Getenv("CTX_ID")
	if sessionID == "" {
		return "", fmt.Errorf("no session ID provided and CTX_ID is not set")
	}
	return sessionID, nil
}
