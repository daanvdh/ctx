// Package cmd implements the command line interface for ctx.
// This file adds support for deleting a session and all of its descendant sessions.

package cmd

import (
	"fmt"
	"os"
)

// Delete handles the `ctx delete <session_id>` command.
// It removes the specified session and any child sessions (recursively),
// along with all stored key/value pairs belonging to those sessions.
func Delete(args []string) int {
	// Handle help flag similar to other commands.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Println(`Usage: ctx delete <session_id>

Delete the specified session, all its child sessions and their variables.`)
			return 0
		}
	}

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "ctx: delete: usage: ctx delete <session_id>\n")
		return 1
	}
	target := args[0]

	a, code := newApp("delete")
	if code != 0 {
		return code
	}
	if err := a.DeleteSessionTree(target); err != nil {
		printErr("delete", err)
		return 1
	}
	return 0
}
