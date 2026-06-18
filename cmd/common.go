package cmd

import (
	"context"
	"fmt"

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
