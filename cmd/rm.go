package cmd

import (
	"context"
	"fmt"
)

func Rm(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx rm [session] <entry>

Remove an entry from a session. entry may contain wildcards (*, ?, [...])
to remove all matching entries, e.g. ctx rm "*trigger_log*".`)
		return nil
	}

	parsed, err := parseRmArgs(args)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.RemoveEntry(ctx, parsed.sessionID, parsed.entry)
}

type rmArgs struct {
	sessionID string
	entry     string
}

func parseRmArgs(args []string) (rmArgs, error) {
	if len(args) != 1 && len(args) != 2 {
		return rmArgs{}, usage("rm", "ctx rm [session] <entry>")
	}
	hasSession := len(args) == 2
	sessionID, err := sessionArg(args, hasSession)
	if err != nil {
		return rmArgs{}, err
	}
	entry := args[0]
	if hasSession {
		entry = args[1]
	}
	return rmArgs{sessionID: sessionID, entry: entry}, nil
}
