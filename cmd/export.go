package cmd

import (
	"context"
	"fmt"
)

func Export(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx export [session_id] [--files-as-paths] [--quiet]

Print all visible key/value pairs as shell export lines, including
CTX_ID. file_ref values are skipped by default; --files-as-paths
exports file_ref values as their path. --quiet suppresses warnings for
keys that aren't valid shell variable names.`)
		return nil
	}

	filesAsPaths := false
	quiet := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--files-as-paths":
			filesAsPaths = true
		case "--quiet":
			quiet = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) > 1 {
		return usage("export", "ctx export [session_id] [--files-as-paths] [--quiet]")
	}

	sessionID, err := sessionArg(filtered, len(filtered) == 1)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	lines, err := a.Export(ctx, sessionID, filesAsPaths, quiet)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}
