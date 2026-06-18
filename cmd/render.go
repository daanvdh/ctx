package cmd

import (
	"context"
	"fmt"
)

// Render handles the `ctx render <session> <key>` command.
// It renders a stored template by substituting `$VAR` placeholders with values from the session context.
func Render(ctx context.Context, args []string) error {
	if len(args) != 2 {
		return usage("render", "ctx render <session> <key>")
	}

	sessionID, key := args[0], args[1]

	a, err := newApp()
	if err != nil {
		return err
	}
	output, err := a.Render(ctx, sessionID, key)
	if err != nil {
		return err
	}

	// Print the rendered result without an extra newline (the value may already contain it).
	fmt.Println(output)
	return nil
}
