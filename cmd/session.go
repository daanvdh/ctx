package cmd

import (
	"context"
	"fmt"
)

func Session(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx session [parent] [name] [--parent <parent-id> | --root]

Create a new session. If name is omitted, a random ID is generated.
By default the session is a child of $CTX_ID, or created at the tree root if CTX_ID is unset.
A parent can be set explicitly as the first positional argument, or via --parent.
--root forces the session to be created at the tree root, ignoring CTX_ID.
--parent and --root are mutually exclusive, as are a positional parent and --parent.
Use "ctx session --help" to display this help message.`)
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
