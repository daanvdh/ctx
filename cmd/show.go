package cmd

import (
	"context"
	"fmt"
)

func Show(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return usage("show", "ctx show [session_id]")
	}

	sessionID, _, err := sessionArg(args, 0)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	lines, err := a.Show(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}
