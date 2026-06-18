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

func sessionArg(args []string, index int) (string, bool, error) {
	if len(args) > index {
		return args[index], true, nil
	}
	sessionID := os.Getenv("CTX_ID")
	if sessionID == "" {
		return "", false, fmt.Errorf("no session ID provided and CTX_ID is not set")
	}
	return sessionID, false, nil
}
