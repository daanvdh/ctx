package cmd

import (
	"context"
	"fmt"
)

func Export(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return usage("export", "ctx export <session_id>")
	}

	sessionID := args[0]

	a, err := newApp()
	if err != nil {
		return err
	}
	lines, err := a.Export(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}
