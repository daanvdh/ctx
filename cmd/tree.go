package cmd

import (
	"fmt"
	"os"

	"ctx/internal/render"
	"ctx/internal/store"
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

	path, err := getCtxPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: tree: %v\n", err)
		return 1
	}

	cf, err := store.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: tree: %v\n", err)
		return 1
	}

	result, err := render.Tree(cf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: tree: %v\n", err)
		return 1
	}

	fmt.Print(result)
	return 0
}
