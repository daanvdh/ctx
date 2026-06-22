package cmd

import (
	"context"
	"fmt"
)

func Get(ctx context.Context, args []string) error {
	if len(args) != 1 && len(args) != 2 {
		return usage("get", "ctx get [session_id] <key>")
	}

	hasSession := len(args) == 2
	sessionID, err := sessionArg(args, hasSession)
	if err != nil {
		return err
	}
	offset := 0
	if hasSession {
		offset = 1
	}
	key := args[offset]

	a, err := newApp()
	if err != nil {
		return err
	}
	value, err := a.GetValue(ctx, sessionID, key)
	if err != nil {
		return err
	}

	fmt.Println(value)
	return nil
}
