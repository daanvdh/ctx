package cmd

import (
	"context"
)

func Set(ctx context.Context, args []string) error {
	if len(args) != 3 {
		return usage("set", "ctx set <session_id> <key> <value>")
	}

	sessionID, key, value := args[0], args[1], args[2]

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.SetValue(ctx, sessionID, key, value)
}
