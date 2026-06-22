package main

import (
	"context"
	"fmt"
	"os"

	"ctx/cmd"
)

func main() {
	if err := cmd.Serve(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ctx-mcp: %v\n", err)
		os.Exit(1)
	}
}
