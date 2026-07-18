// Package cmd contains one entry point per ctx sub-command. Each function
// parses its own arguments and delegates to internal/app.
package cmd

import (
	"fmt"
	"os"

	"ctx/internal/app"
	"ctx/internal/config"
)

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
		sessionID = config.DefaultSession()
	}
	if sessionID == "" {
		return "", fmt.Errorf("no session ID provided, CTX_ID is not set, and no default_session is configured")
	}
	return sessionID, nil
}
