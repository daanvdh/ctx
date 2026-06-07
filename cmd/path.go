package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// getCtxPath returns the path to the ctx.json file in the current working directory.
func getCtxPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(wd, "ctx.json"), nil
}

// Path handles the 'ctx path' command, printing the path to the ctx.json file.
func Path(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ctx: path: unexpected error: %v\n", r)
			os.Exit(1)
		}
	}()

	if len(args) != 0 {
		fmt.Fprintf(os.Stderr, "ctx: path: usage: ctx path\n")
		return 1
	}

	path, err := getCtxPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: path: %v\n", err)
		return 1
	}

	fmt.Println(path)
	return 0
}
