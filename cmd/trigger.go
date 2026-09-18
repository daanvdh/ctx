// Package cmd implements the command line interface for ctx.
// This file adds support for manually firing a trigger template stored in the
// triggers directory.

package cmd

import (
	"context"
	"fmt"
)

// Trigger handles the `ctx trigger <session> <template>` command
// (formerly `ctx execute`, which remains as an alias).
// It loads a template file from $HOME/.config/ctx/triggers/<template>,
// substitutes placeholders ($VAR) using session variables (including ancestors),
// and runs the specified script with the rendered prompt as a quoted argument.
func Trigger(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx trigger <session> <template>

Fire a stored trigger template. The template must be placed in $HOME/.config/ctx/triggers/<template>.
The frontmatter (before the optional --- separator) is YAML. Example:`)
		fmt.Println("    script: pi        # Program invoked with the rendered prompt.")
		fmt.Println(`---
<prompt text possibly containing placeholders like $VAR>`)
		return nil
	}

	if len(args) != 1 && len(args) != 2 {
		return usage("trigger", "ctx trigger [session] <template>")
	}
	hasSession := len(args) == 2
	sessionID, err := sessionArg(args, hasSession)
	if err != nil {
		return err
	}
	offset := 0
	if hasSession {
		offset = 1
	}
	tmplName := args[offset]

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.Execute(ctx, sessionID, tmplName)
}
