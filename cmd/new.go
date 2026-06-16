package cmd

import (
	"fmt"
	"os"

	"time"

	"ctx/internal/model"
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

		var explicitParent *string
		customID := ""

		// Parse arguments: optional custom ID and optional --parent flag.
		for i := 0; i < len(args); {
			arg := args[i]
			if arg == "--parent" {
				if i+1 >= len(args) {
					return fmt.Errorf("missing argument for --parent")
				}
				p := args[i+1]
				explicitParent = &p
				i += 2
				continue
			}
			// Treat any non-flag argument as custom ID.
			if customID == "" {
				customID = arg
			} else {
				return fmt.Errorf("unexpected extra argument: %s", arg)
			}
			i++
		}

		// Determine parent ID based on precedence.
		var parentID *string
		if explicitParent != nil {
			parentID = explicitParent
		} else if env := os.Getenv("CTX_ID"); env != "" {
			pEnv := env
			parentID = &pEnv
		}

		// Validation helper for session IDs.
		isValidID := func(id string) bool {
			if id == "" {
				return false
			}
			for _, r := range id {
				if (r >= 'a' && r <= 'z') ||
					(r >= 'A' && r <= 'Z') ||
					(r >= '0' && r <= '9') ||
					r == '-' || r == '_' {
					continue
				}
				return false
			}
			return true
		}

		var outIDLocal string
		if customID != "" {
			// Validate the provided custom ID.
			if !isValidID(customID) {
				return fmt.Errorf("invalid session ID: %s", customID)
			}
			// Ensure it does not already exist.
			if _, exists := cf.Sessions[customID]; exists {
				return fmt.Errorf("error: session '%s' already exists", customID)
			}
			cf.Sessions[customID] = &model.Session{
				Parent:  parentID,
				Created: time.Now(),
				Data:    make(map[string]string),
			}
			outIDLocal = customID
		} else {
			genID, err := session.New(cf, parentID)
			if err != nil {
				return err
			}
			outIDLocal = genID
		}

		// Save changes.
		if err := store.Save(path, cf); err != nil {
			return err
		}
		outID = outIDLocal
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: new: %v\n", err)
		return 1
	}

	fmt.Println(outID)
	return 0
}
