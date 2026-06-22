package cmd

import (
	"context"
)

func Set(ctx context.Context, args []string) error {
	if len(args) != 2 && len(args) != 3 {
		return usage("set", "ctx set [session_id] <key> <value>")
	}

	sessionID, usedArg, err := sessionArg(args, 0)
	if err != nil {
		return err
	}
	offset := 0
	if usedArg {
		offset = 1
	}
	key, value := args[offset], args[offset+1]

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.SetValue(ctx, sessionID, key, value)
}
