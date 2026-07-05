package cmd

import (
	"context"
	"fmt"
)

func New(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx new [custom_id] [--parent <parent-id>]

Create a new session. If a custom ID is supplied, it is used (must consist of letters, digits, hyphens or underscores). Otherwise an 8‑character hexadecimal ID is generated.
Parent can be set explicitly via --parent flag, or implicitly from the CTX_ID environment variable if present.
Use "ctx new --help" to display this help message.`)
		return nil
	}

	var explicitParent *string
	customID := ""
	for i := 0; i < len(args); {
		arg := args[i]
		if arg == "--parent" {
			if i+1 >= len(args) {
				return fmt.Errorf("missing argument for --parent")
			}
			parent := args[i+1]
			explicitParent = &parent
			i += 2
			continue
		}
		if customID == "" {
			customID = arg
		} else {
			return fmt.Errorf("unexpected extra argument: %s", arg)
		}
		i++
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	outID, err := a.CreateSession(ctx, customID, explicitParent)
	if err != nil {
		return err
	}

	fmt.Println(outID)
	return nil
}
