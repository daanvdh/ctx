package cmd

import (
	"fmt"
	"os"
)

func Get(args []string) int {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "ctx: get: usage: ctx get <session_id> <key>\n")
		return 1
	}

	sessionID, key := args[0], args[1]

	a, code := newApp("get")
	if code != 0 {
		return code
	}
	value, err := a.GetValue(sessionID, key)
	if err != nil {
		printErr("get", err)
		return 1
	}

	fmt.Println(value)
	return 0
}
