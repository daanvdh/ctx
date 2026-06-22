package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"ctx/internal/app"
	"ctx/internal/mcp"
)

func main() {
	httpMode := flag.Bool("http", false, "serve MCP over Streamable HTTP instead of stdio")
	addr := flag.String("addr", "127.0.0.1:7331", "HTTP listen address")
	path := flag.String("path", "/mcp", "HTTP MCP endpoint path")
	allowedOrigins := flag.String("allowed-origins", "", "comma-separated Origin values allowed for browser-originated requests")
	flag.Parse()

	if *httpMode {
		mux := http.NewServeMux()
		mux.Handle(*path, mcp.NewHTTPHandler(app.New, splitCSV(*allowedOrigins)))
		fmt.Fprintf(os.Stderr, "ctx-mcp: listening on http://%s%s\n", *addr, *path)
		if err := http.ListenAndServe(*addr, mux); err != nil {
			fmt.Fprintf(os.Stderr, "ctx-mcp: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := mcp.NewServer(os.Stdin, os.Stdout).Serve(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "ctx-mcp: %v\n", err)
		os.Exit(1)
	}
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
