package cmd

import (
	"fmt"
	"os"
)

func Export(args []string) int {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "ctx: export: usage: ctx export <session_id>\n")
		return 1
	}

	sessionID := args[0]

	a, code := newApp("export")
	if code != 0 {
		return code
	}
	lines, err := a.Export(sessionID)
	if err != nil {
		printErr("export", err)
		return 1
	}
	for _, line := range lines {
		fmt.Println(line)
	}

	return 0
}
