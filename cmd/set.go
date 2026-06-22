package cmd

import (
	"context"
)

func Set(ctx context.Context, args []string) error {
	if len(args) != 2 && len(args) != 3 {
		return usage("set", "ctx set [session_id] <key> <value>")
	}

	hasSession := len(args) == 3
	sessionID, err := sessionArg(args, hasSession)
	if err != nil {
		return err
	}
	offset := 0
	if hasSession {
		offset = 1
	}
	key, value := args[offset], args[offset+1]

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.SetValue(ctx, sessionID, key, value)
}
