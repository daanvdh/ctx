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
	"ctx/internal/config"
	"ctx/internal/mcp"
)

type serveOptions struct {
	httpMode       bool
	addr           string
	path           string
	serverName     string
	allowedOrigins []string
	auth           mcp.AuthConfig
	debug          bool
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
		auth := mcp.NewHTTPAuth(opts.auth)
		mux := http.NewServeMux()
		if auth.Enabled() {
			auth.Register(mux)
		}
		mux.Handle(opts.path, mcp.NewHTTPHandlerWithOptions(app.New, mcp.HTTPOptions{
			AllowedOrigins: opts.allowedOrigins,
			ServerName:     opts.serverName,
			Auth:           auth,
		}))
		handler := http.Handler(mux)
		if opts.debug {
			handler = mcp.NewAccessLogHandler(handler, os.Stderr)
		}
		fmt.Fprintf(os.Stderr, "ctx: serve: listening on http://%s%s\n", opts.addr, opts.path)
		if auth.Enabled() {
			fmt.Fprintf(os.Stderr, "ctx: serve: auth enabled; publish http://%s, not only http://%s%s, so OAuth discovery routes are reachable\n", opts.addr, opts.addr, opts.path)
		}
		if opts.debug {
			fmt.Fprintln(os.Stderr, "ctx: serve: debug access logging enabled")
		}
		return http.ListenAndServe(opts.addr, handler)
	}

	return mcp.NewServer(os.Stdin, os.Stdout).Serve(ctx)
}

func parseServeArgs(args []string) (*serveOptions, error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return parseServeArgsWithSettings(args, config.Settings{})
		}
	}
	settings, err := config.LoadSettings()
	if err != nil {
		return nil, err
	}
	return parseServeArgsWithSettings(args, settings)
}

func parseServeArgsWithSettings(args []string, settings config.Settings) (*serveOptions, error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`Usage: ctx serve [--http] [--addr <addr>] [--path <path>] [--name <name>] [--allowed-origins <origins>] [--debug]

Serve the ctx MCP server.

By default, ctx serve uses MCP stdio transport. Use --http to serve Streamable HTTP.`)
			return nil, nil
		}
	}

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	defaultAddr := stringDefault(settings.MCPHTTPAddr, "127.0.0.1:7331")
	defaultPath := pathDefault(settings.MCPHTTPPath, "/mcp")
	defaultName := stringDefault(settings.MCPServerName, "ctx-mcp")
	defaultOrigins := strings.Join(settings.MCPAllowedOrigins, ",")
	httpMode := fs.Bool("http", false, "serve MCP over Streamable HTTP instead of stdio")
	addr := fs.String("addr", defaultAddr, "HTTP listen address")
	path := fs.String("path", defaultPath, "HTTP MCP endpoint path")
	serverName := fs.String("name", defaultName, "MCP server name reported to clients")
	allowedOrigins := fs.String("allowed-origins", defaultOrigins, "comma-separated Origin values allowed for browser-originated requests")
	debug := fs.Bool("debug", envBool("CTX_MCP_DEBUG"), "log HTTP MCP and OAuth requests to stderr")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() != 0 {
		return nil, usage("serve", "ctx serve [--http] [--addr <addr>] [--path <path>] [--name <name>] [--allowed-origins <origins>] [--debug]")
	}

	resolvedPath := pathDefault(*path, "/mcp")
	resolvedName := stringDefault(*serverName, "ctx-mcp")
	return &serveOptions{
		httpMode:       *httpMode,
		addr:           *addr,
		path:           resolvedPath,
		serverName:     resolvedName,
		allowedOrigins: splitCSV(*allowedOrigins),
		auth: mcp.AuthConfig{
			ClientID:          envDefault("CTX_MCP_CLIENT_ID", settings.MCPOAuthClientID),
			ClientSecret:      envDefault("CTX_MCP_CLIENT_SECRET", settings.MCPOAuthClientSecret),
			StaticBearerToken: envDefault("CTX_MCP_TOKEN", settings.MCPToken),
			PublicURL:         envDefault("CTX_MCP_PUBLIC_URL", settings.MCPPublicURL),
			ResourcePath:      resolvedPath,
			ServerName:        resolvedName,
		},
		debug: *debug,
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

func stringDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func pathDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
