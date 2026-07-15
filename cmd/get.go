package cmd

import (
	"context"
	"fmt"
)

func Get(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx get [session_id] <key> [--raw] [--allow-missing]

Get a key's value from a session (or an ancestor), rendering $VAR
placeholders by default. file_ref entries reference files living outside
ctx and are never rendered, since that content wasn't authored with ctx's
$VAR syntax in mind.

--raw returns the value unrendered instead: for file_ref entries this is
the referenced path itself; for string and doc entries it's the stored
value with no placeholder substitution.

--allow-missing leaves unresolved $VAR placeholders unchanged instead of
failing.`)
		return nil
	}

	raw := false
	allowMissing := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--raw":
			raw = true
		case "--allow-missing":
			allowMissing = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) != 1 && len(filtered) != 2 {
		return usage("get", "ctx get [session_id] <key> [--raw] [--allow-missing]")
	}

	hasSession := len(filtered) == 2
	sessionID, err := sessionArg(filtered, hasSession)
	if err != nil {
		return err
	}
	offset := 0
	if hasSession {
		offset = 1
	}
	key := filtered[offset]

	a, err := newApp()
	if err != nil {
		return err
	}
	var value string
	if raw {
		value, err = a.GetRaw(ctx, sessionID, key)
	} else {
		value, err = a.GetRendered(ctx, sessionID, key, allowMissing)
	}
	if err != nil {
		return err
	}

	fmt.Println(value)
	return nil
}
