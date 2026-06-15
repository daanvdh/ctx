package cmd

import (
	"fmt"
	"os"

	"ctx/internal/session"
	"ctx/internal/store"
)

func Get(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ctx: get: unexpected error: %v\n", r)
			os.Exit(1)
		}
	}()

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "ctx: get: usage: ctx get <session_id> <key>\n")
		return 1
	}

	sessionID, key := args[0], args[1]

	path, err := getCtxPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: get: %v\n", err)
		return 1
	}

	cf, err := store.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: get: %v\n", err)
		return 1
	}

	value, err := session.Get(cf, sessionID, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: get: %v\n", err)
		return 1
	}

	fmt.Println(value)
	return 0
}
