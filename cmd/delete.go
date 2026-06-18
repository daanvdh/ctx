// Package cmd implements the command line interface for ctx.
// This file adds support for deleting a session and all of its descendant sessions.

package cmd

import (
	"context"
	"fmt"
)

// Delete handles the `ctx delete <session_id>` command.
// It removes the specified session and any child sessions (recursively),
// along with all stored key/value pairs belonging to those sessions.
func Delete(ctx context.Context, args []string) error {
	// Handle help flag similar to other commands.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Println(`Usage: ctx delete <session_id>

Delete the specified session, all its child sessions and their variables.`)
			return nil
		}
	}

	if len(args) > 1 {
		return usage("delete", "ctx delete [session_id]")
	}
	target, _, err := sessionArg(args, 0)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.DeleteSessionTree(ctx, target)
}
