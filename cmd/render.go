package cmd

import (
	"fmt"
	"os"
)

// Render handles the `ctx render <session> <key>` command.
// It renders a stored template by substituting `$VAR` placeholders with values from the session context.
func Render(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ctx: render: unexpected error: %v\n", r)
			os.Exit(1)
		}
	}()

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "ctx: render: usage: ctx render <session> <key>\n")
		return 1
	}

	sessionID, key := args[0], args[1]

	a, code := newApp("render")
	if code != 0 {
		return code
	}
	output, err := a.Render(sessionID, key)
	if err != nil {
		printErr("render", err)
		return 1
	}

	// Print the rendered result without an extra newline (the value may already contain it).
	fmt.Println(output)
	return 0
}
