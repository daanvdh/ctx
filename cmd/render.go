package cmd

import (
	"context"
	"fmt"
)

// Render handles the `ctx render <session> <key>` command.
// It renders a stored template by substituting `$VAR` placeholders with values from the session context.
func Render(ctx context.Context, args []string) error {
	ignoreMissing := false
	filtered := args[:0]
	for _, arg := range args {
		switch arg {
		case "--ignore-missing":
			ignoreMissing = true
		default:
			filtered = append(filtered, arg)
		}
	}
	args = filtered

	if len(args) != 1 && len(args) != 2 {
		return usage("render", "ctx render [--ignore-missing] [session] <key>")
	}

	sessionID, usedArg, err := sessionArg(args, 0)
	if err != nil {
		return err
	}
	offset := 0
	if usedArg {
		offset = 1
	}
	key := args[offset]

	a, err := newApp()
	if err != nil {
		return err
	}
	output, err := a.Render(ctx, sessionID, key, ignoreMissing)
	if err != nil {
		return err
	}

	// Print the rendered result without an extra newline (the value may already contain it).
	fmt.Println(output)
	return nil
}
