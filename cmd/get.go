package cmd

import (
	"context"
	"fmt"
)

func Get(ctx context.Context, args []string) error {
	if len(args) != 2 {
		return usage("get", "ctx get <session_id> <key>")
	}

	sessionID, key := args[0], args[1]

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
