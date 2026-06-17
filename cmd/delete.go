// Package cmd implements the command line interface for ctx.
// This file adds support for deleting a session and all of its descendant sessions.

package cmd

import (
    "fmt"
    "os"

    "ctx/internal/store"
)

// Delete handles the `ctx delete <session_id>` command.
// It removes the specified session and any child sessions (recursively),
// along with all stored key/value pairs belonging to those sessions.
func Delete(args []string) int {
    // Handle help flag similar to other commands.
    for _, a := range args {
        if a == "--help" || a == "-h" {
            fmt.Println(`Usage: ctx delete <session_id>

Delete the specified session, all its child sessions and their variables.`)
            return 0
        }
    }

    if len(args) != 1 {
        fmt.Fprintf(os.Stderr, "ctx: delete: usage: ctx delete <session_id>\n")
        return 1
    }
    target := args[0]

    path, err := getCtxPath()
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: delete: %v\n", err)
        return 1
    }

    // Acquire lock and perform deletion.
    err = store.WithLock(path, func() error {
        cf, loadErr := store.Load(path)
        if loadErr != nil {
            return loadErr
        }
        // Ensure the target session exists.
        if _, ok := cf.Sessions[target]; !ok {
            return fmt.Errorf("session %s not found", target)
        }

        // Build a map from parent ID to its child IDs for fast lookup.
        childrenMap := make(map[string][]string)
        for id, sess := range cf.Sessions {
            if sess != nil && sess.Parent != nil {
                pid := *sess.Parent
                childrenMap[pid] = append(childrenMap[pid], id)
            }
        }

        // Collect all sessions to delete: target plus its descendants.
        var toDelete []string
        stack := []string{target}
        visited := make(map[string]bool)
        for len(stack) > 0 {
            cur := stack[len(stack)-1]
            stack = stack[:len(stack)-1]
            if visited[cur] {
                continue
            }
            visited[cur] = true
            toDelete = append(toDelete, cur)
            // Append children of the current session.
            for _, child := range childrenMap[cur] {
                stack = append(stack, child)
            }
        }

        // Remove collected sessions from the map.
        for _, id := range toDelete {
            delete(cf.Sessions, id)
        }

        // Persist updated context file.
        return store.Save(path, cf)
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: delete: %v\n", err)
        return 1
    }
    return 0
}
