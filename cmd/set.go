package cmd

import (
	"fmt"
	"os"

	"ctx/internal/session"
	"ctx/internal/store"
)

func Set(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ctx: set: unexpected error: %v\n", r)
			os.Exit(1)
		}
	}()

	if len(args) != 3 {
		fmt.Fprintf(os.Stderr, "ctx: set: usage: ctx set <session_id> <key> <value>\n")
		return 1
	}

	sessionID, key, value := args[0], args[1], args[2]

	path, err := getCtxPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: set: %v\n", err)
		return 1
	}

	err = store.WithLock(path, func() error {
		cf, loadErr := store.Load(path)
		if loadErr != nil {
			return loadErr
		}

		if err := session.Set(cf, sessionID, key, value); err != nil {
			return err
		}

		return store.Save(path, cf)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: set: %v\n", err)
		return 1
	}

	return 0
}
