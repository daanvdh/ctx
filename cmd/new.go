package cmd

import (
	"fmt"
)

func New(args []string) int {
	// If --help flag is present, show usage and exit.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Println(`Usage: ctx new [custom_id] [--parent <parent-id>]

Create a new session. If a custom ID is supplied, it is used (must consist of letters, digits, hyphens or underscores). Otherwise an 8‑character hexadecimal ID is generated.
Parent can be set explicitly via --parent flag, or implicitly from the CTX_ID environment variable if present.
Use "ctx new --help" to display this help message.`)
			return 0
		}
	}

	var explicitParent *string
	customID := ""
	for i := 0; i < len(args); {
		arg := args[i]
		if arg == "--parent" {
			if i+1 >= len(args) {
				printErr("new", fmt.Errorf("missing argument for --parent"))
				return 1
			}
			parent := args[i+1]
			explicitParent = &parent
			i += 2
			continue
		}
		if customID == "" {
			customID = arg
		} else {
			printErr("new", fmt.Errorf("unexpected extra argument: %s", arg))
			return 1
		}
		i++
	}

	a, code := newApp("new")
	if code != 0 {
		return code
	}
	outID, err := a.CreateSession(customID, explicitParent)
	if err != nil {
		printErr("new", err)
		return 1
	}

	fmt.Println(outID)
	return 0
}
