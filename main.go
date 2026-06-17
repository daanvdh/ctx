package main

import (
	"fmt"
	"os"

	"ctx/cmd"
)

func main() {
	// If no command is provided or the user asks for help, display usage information.
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		cmd.Help(nil)
		os.Exit(0)
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
	case "delete":
		exitCode = cmd.Delete(args)
	case "help":
		exitCode = cmd.Help(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}

	os.Exit(exitCode)
}
