package cmd

import (
	"context"
	"fmt"
)

func Get(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx get [session_id] <key> [--raw] [--path|--preview]

Get a key's value from a session (or an ancestor), rendering $VAR
placeholders by default. --raw returns the unrendered value. --path
writes doc content to a temp file and prints its path (always raw);
--preview prints a truncated preview instead of the full value.`)
		return nil
	}

	asPath := false
	preview := false
	raw := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--path":
			asPath = true
		case "--preview":
			preview = true
		case "--raw":
			raw = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if asPath && preview {
		return fmt.Errorf("--path and --preview are mutually exclusive")
	}
	if len(filtered) != 1 && len(filtered) != 2 {
		return usage("get", "ctx get [session_id] <key> [--raw] [--path|--preview]")
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
	switch {
	case asPath:
		value, err = a.GetPath(ctx, sessionID, key)
	case preview:
		value, err = a.GetPreview(ctx, sessionID, key, raw)
	case raw:
		value, err = a.GetValue(ctx, sessionID, key)
	default:
		value, err = a.GetRendered(ctx, sessionID, key)
	}
	if err != nil {
		return err
	}

	fmt.Println(value)
	return nil
}
