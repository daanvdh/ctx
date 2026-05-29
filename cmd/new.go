package cmd

import (
	"fmt"
	"os"

	"ctx/internal/session"
	"ctx/internal/store"
)

func New(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ctx: new: unexpected error: %v\n", r)
			os.Exit(1)
		}
	}()

	path, err := getCtxPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: new: %v\n", err)
		return 1
	}

	var outID string
	err = store.WithLock(path, func() error {
		cf, loadErr := store.Load(path)
		if loadErr != nil {
			return loadErr
		}

		var parentID *string
		if len(args) > 0 && args[0] != "" {
			pid := args[0]
			parentID = &pid
		}

		newID, newErr := session.New(cf, parentID)
		if newErr != nil {
			return newErr
		}

		if err := store.Save(path, cf); err != nil {
			return err
		}

		outID = newID
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: new: %v\n", err)
		return 1
	}

	fmt.Println(outID)
	return 0
}
