package cmd

import (
	"fmt"
	"os"
)

func Set(args []string) int {
	if len(args) != 3 {
		fmt.Fprintf(os.Stderr, "ctx: set: usage: ctx set <session_id> <key> <value>\n")
		return 1
	}

	sessionID, key, value := args[0], args[1], args[2]

	a, code := newApp("set")
	if code != 0 {
		return code
	}
	if err := a.SetValue(sessionID, key, value); err != nil {
		printErr("set", err)
		return 1
	}

	return 0
}
