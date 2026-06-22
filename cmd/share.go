package cmd

import "context"

func Share(ctx context.Context, args []string) error {
	if len(args) != 2 {
		return usage("share", "ctx share <from> <to>")
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.ShareContext(ctx, args[0], args[1])
}
