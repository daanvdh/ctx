// Package cmd implements the command line interface for ctx.
// This file adds support for executing a template stored in the triggers directory.

package cmd

import (
	"fmt"
	"os"
)

// Execute handles the `ctx execute <session> <template>` command.
// It loads a template file from $HOME/.config/ctx/triggers/<template>,
// substitutes placeholders ($VAR) using session variables (including ancestors),
// and runs the specified command with the rendered prompt as a quoted argument.
func Execute(args []string) int {
	// Show help if requested.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Println(`Usage: ctx execute <session> <template>

Execute a stored trigger template. The template must be placed in $HOME/.config/ctx/triggers/<template>.
The first line should define the command to run, e.g.:`)
			fmt.Println("    command=pi        # Program invoked with the rendered prompt.")
			fmt.Println(`---
<prompt text possibly containing placeholders like $VAR>`)
			return 0
		}
	}

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "ctx: execute: usage: ctx execute <session> <template>\n")
		return 1
	}
	sessionID := args[0]
	tmplName := args[1]

	a, code := newApp("execute")
	if code != 0 {
		return code
	}
	if err := a.Execute(sessionID, tmplName); err != nil {
		printErr("execute", err)
		return 1
	}
	return 0
}
