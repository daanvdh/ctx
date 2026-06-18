// Package cmd implements the command line interface for ctx.
// This file adds support for executing a template stored in the triggers directory.

package cmd

import (
	"context"
	"fmt"
)

// Execute handles the `ctx execute <session> <template>` command.
// It loads a template file from $HOME/.config/ctx/triggers/<template>,
// substitutes placeholders ($VAR) using session variables (including ancestors),
// and runs the specified command with the rendered prompt as a quoted argument.
func Execute(ctx context.Context, args []string) error {
	// Show help if requested.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Println(`Usage: ctx execute <session> <template>

Execute a stored trigger template. The template must be placed in $HOME/.config/ctx/triggers/<template>.
The first line should define the command to run, e.g.:`)
			fmt.Println("    command=pi        # Program invoked with the rendered prompt.")
			fmt.Println(`---
<prompt text possibly containing placeholders like $VAR>`)
			return nil
		}
	}

	if len(args) != 1 && len(args) != 2 {
		return usage("execute", "ctx execute [session] <template>")
	}
	sessionID, usedArg, err := sessionArg(args, 0)
	if err != nil {
		return err
	}
	offset := 0
	if usedArg {
		offset = 1
	}
	tmplName := args[offset]

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.Execute(ctx, sessionID, tmplName)
}
