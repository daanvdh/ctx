package cmd

import (
	"context"
	"fmt"
)

func Export(ctx context.Context, args []string) error {
	includeDocs := false
	filesAsPaths := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--include-docs":
			includeDocs = true
		case "--files-as-paths":
			filesAsPaths = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) > 1 {
		return usage("export", "ctx export [session_id] [--include-docs] [--files-as-paths]")
	}

	sessionID, err := sessionArg(filtered, len(filtered) == 1)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	lines, err := a.Export(ctx, sessionID, includeDocs, filesAsPaths)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}
