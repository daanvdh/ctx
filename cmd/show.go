package cmd

import (
	"context"
	"fmt"

	"ctx/internal/app"
)

func Show(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx show [session_id] [--full] [--render]

Show all entries visible to a session. By default, doc and file values
are previewed (first characters only). --full shows the complete
value instead. --render additionally substitutes $VAR placeholders
(recursively, up to depth 10) before display. Both flags can be
combined.`)
		return nil
	}

	opts := app.ShowOptions{}
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--full":
			opts.Full = true
		case "--render":
			opts.Render = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) > 1 {
		return usage("show", "ctx show [session_id] [--full] [--render]")
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
