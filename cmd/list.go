package cmd

import (
	"context"
	"fmt"

	"ctx/internal/app"
)

func List(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx list [session_id] [--full] [--raw]

List all entries visible to a session, rendering $VAR placeholders
(recursively, up to depth 10) by default. Values are previewed (first
characters only) unless --full shows the complete value. --raw skips
rendering and shows unresolved values. Both flags can be combined.`)
		return nil
	}

	raw := false
	opts := app.ShowOptions{}
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--full":
			opts.Full = true
		case "--raw":
			raw = true
		default:
			filtered = append(filtered, arg)
		}
	}
	opts.Render = !raw
	if len(filtered) > 1 {
		return usage("list", "ctx list [session_id] [--full] [--raw]")
	}

	sessionID, err := sessionArg(filtered, len(filtered) == 1)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	lines, err := a.Show(ctx, sessionID, opts)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}
