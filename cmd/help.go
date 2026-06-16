package cmd

import (
    "fmt"
)

// Help prints usage information for the ctx command line tool.
// It lists all available commands and a short description of each.
func Help(_ []string) int {
    fmt.Println("ctx — Agent Context Manager")
    fmt.Println()
    fmt.Println("Usage: ctx <command> [args...]")
    fmt.Println()
    fmt.Println("Commands:")
    fmt.Println("  new [parent]                Create a new session")
    fmt.Println("  set <session> <key> <value> Set a key in a session")
    fmt.Println("  get <session> <key>         Get a key from a session")
    fmt.Println("  export <session>            Export all visible keys")
    fmt.Println("  tree                        Show the session tree")
    fmt.Println("  help                        Show this help message")
    return 0
}
