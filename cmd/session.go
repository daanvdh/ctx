package cmd

import (
	"context"
	"fmt"
)

func Session(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "rm" {
		return sessionRm(ctx, args[1:])
	}

	if helpRequested(args) {
		fmt.Println(`Usage: ctx session [parent] [name] [--parent <parent-id> | --root]
       ctx session rm <session> [--recursive] [--no-var] [--no-child]

Create a new session. If name is omitted, a random ID is generated.
By default the session is a child of $CTX_ID, or created at the tree root if CTX_ID is unset.
A parent can be set explicitly as the first positional argument, or via --parent.
--root forces the session to be created at the tree root, ignoring CTX_ID.
--parent and --root are mutually exclusive, as are a positional parent and --parent.
Use "ctx session --help" to display this help message.

"ctx session rm" deletes the specified session and its variables. It fails if the
session has child sessions, unless --recursive is given, in which case all
descendants are deleted too, bottom-up. --no-var and --no-child skip nodes
that still have variables or children instead of deleting them.
Use "ctx session rm --help" for details.`)
		return nil
	}

	parsed, err := parseSessionArgs(args)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	outID, err := a.CreateSession(ctx, parsed.name, parsed.parent, parsed.root)
	if err != nil {
		return err
	}

	fmt.Println(outID)
	return nil
}

type sessionArgs struct {
	name   string
	parent *string
	root   bool
}

func parseSessionArgs(args []string) (sessionArgs, error) {
	var flagParent *string
	root := false
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--parent":
			if i+1 >= len(args) {
				return sessionArgs{}, fmt.Errorf("missing argument for --parent")
			}
			p := args[i+1]
			flagParent = &p
			i++
		case "--root":
			root = true
		default:
			positional = append(positional, args[i])
		}
	}

	if flagParent != nil && root {
		return sessionArgs{}, fmt.Errorf("--parent and --root cannot be used together")
	}

	var positionalParent *string
	name := ""
	switch len(positional) {
	case 0:
	case 1:
		name = positional[0]
	case 2:
		p := positional[0]
		positionalParent = &p
		name = positional[1]
	default:
		return sessionArgs{}, usage("session", "ctx session [parent] [name] [--parent <parent-id> | --root]")
	}

	if positionalParent != nil {
		if flagParent != nil {
			return sessionArgs{}, fmt.Errorf("parent specified both positionally and via --parent")
		}
		if root {
			return sessionArgs{}, fmt.Errorf("parent specified both positionally and via --root")
		}
		flagParent = positionalParent
	}

	return sessionArgs{name: name, parent: flagParent, root: root}, nil
}

func sessionRm(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx session rm <session> [--recursive] [--no-var] [--no-child]

Delete the specified session and its variables.

Non-recursive (default): fails if the session has child sessions.
--no-var additionally fails if the session has variables.
--no-child is accepted but ignored (already the default).

--recursive deletes the session and all its descendants, bottom-up.
--no-var skips (keeps) any node in the subtree that has variables.
--no-child skips (keeps) any node that still has children after the
bottom-up pass. Combining both prunes: only fully empty nodes are removed.`)
		return nil
	}

	target, recursive, noVar, noChild, err := parseSessionRmArgs(args)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.DeleteSession(ctx, target, recursive, noVar, noChild)
}

func parseSessionRmArgs(args []string) (target string, recursive, noVar, noChild bool, err error) {
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--recursive":
			recursive = true
		case "--no-var":
			noVar = true
		case "--no-child":
			noChild = true
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 {
		return "", false, false, false, usage("session rm", "ctx session rm <session> [--recursive] [--no-var] [--no-child]")
	}

	target, err = sessionArg(positional, len(positional) == 1)
	if err != nil {
		return "", false, false, false, err
	}
	return target, recursive, noVar, noChild, nil
}
