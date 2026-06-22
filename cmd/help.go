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
	fmt.Println("  new [custom_id] [--parent <parent-id>]   Create a new session. If <custom_id> is omitted an auto‑generated ID is used.")
	fmt.Println("                                              Use --parent to set the parent explicitly; otherwise CTX_ID env variable is used.")
	fmt.Println("  set <session> <key> <value>                Set a key in a session")
	fmt.Println("  get <session> <key>                        Get a key from a session")
	fmt.Println("  share <from> <to>                          Share one session's context with another")
	fmt.Println("  export <session>                           Export all visible keys")
	fmt.Println("  tree                                      Show the session tree")
	fmt.Println("      --format text|json                   Choose tree output format")
	fmt.Println("  render <session> <key>		Render a stored template with placeholders using session variables")
	fmt.Println("  delete <session>                          Delete a session and its descendants (including variables)")
	fmt.Println("  execute <session> <template>		Execute a stored trigger template using the defined command and placeholders")
	fmt.Println("  serve [--http]                            Serve the ctx MCP server")
	fmt.Println("  help                                      Show this help message")
	return nil
}
