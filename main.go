package main

import (
	"context"
	"fmt"
	"os"

	"ctx/cmd"
)

var version = "dev"

func main() {
	commands := map[string]cmd.Command{
		"session": {Name: "session", Run: cmd.Session},
		"set":     {Name: "set", Run: cmd.Set},
		"rm":      {Name: "rm", Run: cmd.Rm},
		"get":     {Name: "get", Run: cmd.Get},
		"export":  {Name: "export", Run: cmd.Export},
		"list":    {Name: "list", Run: cmd.List},
		"ls":      {Name: "list", Run: cmd.List},
		"share":   {Name: "share", Run: cmd.Share},
		"tree":    {Name: "tree", Run: cmd.Tree},
		"trigger": {Name: "trigger", Run: cmd.Trigger},
		"execute": {Name: "trigger", Run: cmd.Trigger},
		"serve":   {Name: "serve", Run: cmd.Serve},
		"help":    {Name: "help", Run: cmd.Help},
	}

	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		if err := cmd.Help(context.Background(), nil); err != nil {
			fmt.Fprintf(os.Stderr, "ctx: help: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if os.Args[1] == "--version" || os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	name := os.Args[1]
	command, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "ctx: unknown command: %s\n", name)
		os.Exit(1)
	}

	if err := command.Run(context.Background(), os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "ctx: %s: %v\n", command.Name, err)
		os.Exit(1)
	}
}
