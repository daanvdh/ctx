package cmd

import (
	"context"
	"fmt"
	"strings"
)

func Tree(ctx context.Context, args []string) error {
	format := "text"
	for i := 0; i < len(args); {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) {
				return fmt.Errorf("missing argument for --format")
			}
			format = args[i+1]
			i += 2
		case "--json":
			format = "json"
			i++
		case "--help", "-h":
			fmt.Println(`Usage: ctx tree [--format text|json]

Show the session tree.`)
			return nil
		default:
			return usage("tree", "ctx tree [--format text|json]")
		}
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	result, err := a.Tree(ctx, strings.ToLower(format))
	if err != nil {
		return err
	}

	fmt.Print(result)
	return nil
}
