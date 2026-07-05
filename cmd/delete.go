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
	if helpRequested(args) {
		fmt.Println(`Usage: ctx delete <session_id>

Delete the specified session, all its child sessions and their variables.`)
		return nil
	}

	if len(args) > 1 {
		return usage("delete", "ctx delete [session_id]")
	}
	target, err := sessionArg(args, len(args) == 1)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.DeleteSessionTree(ctx, target)
}
