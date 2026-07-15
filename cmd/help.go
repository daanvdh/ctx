package cmd

import (
	"context"
	"fmt"
)

// Help prints usage information for the ctx command line tool.
// It lists all available commands and a short description of each.
func Help(_ context.Context, _ []string) error {
	fmt.Println("ctx — Agent Context Manager")
	fmt.Println()
	fmt.Println("Usage: ctx <command> [args...]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  session [parent] [name] [--parent <p> | --root]   Create a new session. If name is omitted an auto‑generated ID is used.")
	fmt.Println("                                              Parent defaults to CTX_ID (or root); --parent overrides it, --root forces the tree root.")
	fmt.Println("  session rm <session> [--recursive] [--no-var] [--no-child]")
	fmt.Println("                                              Delete a session and its variables. Fails if it has children unless --recursive.")
	fmt.Println("                                              --no-var/--no-child skip (recursive) or fail on (non-recursive) nodes with variables/children.")
	fmt.Println("  set <session> <key> <value>                Set a key in a session")
	fmt.Println("  rm [session] <entry>                       Remove an entry from a session")
	fmt.Println("  get <session> <key> [--raw] [--allow-missing]  Get a key from a session, rendering placeholders by default")
	fmt.Println("  share <from> <to>                          Share one session's context with another")
	fmt.Println("  export <session>                           Export all visible keys")
	fmt.Println("  list [session_id] [--full] [--raw]         List all visible entries, rendered by default (alias: ls)")
	fmt.Println("  tree [session_id] [-a]                     Show the session tree, scoped to session_id (or CTX_ID) unless -a is given")
	fmt.Println("      --format text|json                   Choose tree output format")
	fmt.Println("  execute <session> <template>		Execute a stored trigger template using the defined script and placeholders")
	fmt.Println("  tick                                      Run every trigger whose schedule matches now")
	fmt.Println("  serve [--http]                            Serve the ctx MCP server")
	fmt.Println("  help                                      Show this help message")
	fmt.Println()
	fmt.Println("Run any command with --help for details, e.g. ctx list --help")
	return nil
}
