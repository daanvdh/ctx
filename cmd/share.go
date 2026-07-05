package cmd

import (
	"context"
	"fmt"
)

func Share(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx share <from> <to>

Make <from> and its ancestors visible to <to>, like an extra parent.
This is an ongoing link (not a one-time copy): later changes to
<from>'s entries are also visible to <to>.`)
		return nil
	}
	if len(args) != 2 {
		return usage("share", "ctx share <from> <to>")
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.ShareContext(ctx, args[0], args[1])
}
