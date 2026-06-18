package cmd

import (
    "fmt"
    "os"

    "ctx/internal/render"
    "ctx/internal/store"
)

// Render handles the `ctx render <session> <key>` command.
// It renders a stored template by substituting `$VAR` placeholders with values from the session context.
func Render(args []string) int {
    defer func() {
        if r := recover(); r != nil {
            fmt.Fprintf(os.Stderr, "ctx: render: unexpected error: %v\n", r)
            os.Exit(1)
        }
    }()

    if len(args) != 2 {
        fmt.Fprintf(os.Stderr, "ctx: render: usage: ctx render <session> <key>\n")
        return 1
    }

    sessionID, key := args[0], args[1]

    path, err := getCtxPath()
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: render: %v\n", err)
        return 1
    }

    cf, err := store.Load(path)
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: render: %v\n", err)
        return 1
    }

    output, err := render.Render(cf, sessionID, key)
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: render: %v\n", err)
        return 1
    }

    // Print the rendered result without an extra newline (the value may already contain it).
    fmt.Println(output)
    return 0
}
