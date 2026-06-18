package cmd

import (
	"fmt"
	"os"
)

func Tree(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ctx: tree: unexpected error: %v\n", r)
			os.Exit(1)
		}
	}()

	if len(args) != 0 {
		fmt.Fprintf(os.Stderr, "ctx: tree: usage: ctx tree\n")
		return 1
	}

	a, code := newApp("tree")
	if code != 0 {
		return code
	}
	result, err := a.Tree()
	if err != nil {
		printErr("tree", err)
		return 1
	}

	fmt.Print(result)
	return 0
}
