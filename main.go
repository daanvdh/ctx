package main

import (
	"fmt"
	"os"

	"ctx/cmd"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: ctx <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  new [parent]    Create a new session\n")
		fmt.Fprintf(os.Stderr, "  set <session> <key> <value>  Set a key in a session\n")
		fmt.Fprintf(os.Stderr, "  get <session> <key>  Get a key from a session\n")
		fmt.Fprintf(os.Stderr, "  export <session>  Export all visible keys\n")
		fmt.Fprintf(os.Stderr, "  tree            Show the session tree\n")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	var exitCode int
	switch command {
	case "new":
		exitCode = cmd.New(args)
	case "set":
		exitCode = cmd.Set(args)
	case "get":
		exitCode = cmd.Get(args)
	case "export":
		exitCode = cmd.Export(args)
	case "tree":
		exitCode = cmd.Tree(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}

	os.Exit(exitCode)
}
