package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func Tree(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx tree [session_id] [-a] [--format text|json]

Show the session tree. If session_id is omitted, CTX_ID is used.
With -a (or no session_id and no CTX_ID), show the full tree of all sessions.`)
		return nil
	}

	parsed, err := parseTreeArgs(args)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	result, err := a.Tree(ctx, parsed.format, parsed.sessionID)
	if err != nil {
		return err
	}

	fmt.Print(result)
	return nil
}

type treeArgs struct {
	sessionID string
	format    string
}

func parseTreeArgs(args []string) (treeArgs, error) {
	format := "text"
	all := false
	sessionID := ""
	hasSessionID := false

	for i := 0; i < len(args); {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) {
				return treeArgs{}, fmt.Errorf("missing argument for --format")
			}
			format = args[i+1]
			i += 2
		case "--json":
			format = "json"
			i++
		case "-a", "--all":
			all = true
			i++
		default:
			if hasSessionID || strings.HasPrefix(args[i], "-") {
				return treeArgs{}, usage("tree", "ctx tree [session_id] [-a] [--format text|json]")
			}
			sessionID = args[i]
			hasSessionID = true
			i++
		}
	}

	if !hasSessionID {
		sessionID = os.Getenv("CTX_ID")
	}
	if all {
		sessionID = ""
	}

	return treeArgs{sessionID: sessionID, format: strings.ToLower(format)}, nil
}
