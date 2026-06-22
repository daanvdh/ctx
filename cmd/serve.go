package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"ctx/internal/app"
	"ctx/internal/mcp"
)

type serveOptions struct {
	httpMode       bool
	addr           string
	path           string
	allowedOrigins []string
}

func Serve(ctx context.Context, args []string) error {
	opts, err := parseServeArgs(args)
	if err != nil {
		return err
	}
	if opts == nil {
		return nil
	}

	if opts.httpMode {
		mux := http.NewServeMux()
		mux.Handle(opts.path, mcp.NewHTTPHandler(app.New, opts.allowedOrigins))
		fmt.Fprintf(os.Stderr, "ctx: serve: listening on http://%s%s\n", opts.addr, opts.path)
		return http.ListenAndServe(opts.addr, mux)
	}

	return mcp.NewServer(os.Stdin, os.Stdout).Serve(ctx)
}

func parseServeArgs(args []string) (*serveOptions, error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`Usage: ctx serve [--http] [--addr <addr>] [--path <path>] [--allowed-origins <origins>]

Serve the ctx MCP server.

By default, ctx serve uses MCP stdio transport. Use --http to serve Streamable HTTP.`)
			return nil, nil
		}
	}

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	httpMode := fs.Bool("http", false, "serve MCP over Streamable HTTP instead of stdio")
	addr := fs.String("addr", "127.0.0.1:7331", "HTTP listen address")
	path := fs.String("path", "/mcp", "HTTP MCP endpoint path")
	allowedOrigins := fs.String("allowed-origins", "", "comma-separated Origin values allowed for browser-originated requests")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() != 0 {
		return nil, usage("serve", "ctx serve [--http] [--addr <addr>] [--path <path>] [--allowed-origins <origins>]")
	}

	return &serveOptions{
		httpMode:       *httpMode,
		addr:           *addr,
		path:           *path,
		allowedOrigins: splitCSV(*allowedOrigins),
	}, nil
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
