package cmd

import (
	"testing"

	"ctx/internal/config"
)

func TestParseServeArgsDefaultsToStdio(t *testing.T) {
	opts, err := parseServeArgsWithSettings(nil, config.Settings{})
	if err != nil {
		t.Fatalf("parseServeArgs error: %v", err)
	}
	if opts == nil {
		t.Fatal("parseServeArgs returned nil options")
	}
	if opts.httpMode {
		t.Fatal("httpMode = true, want false")
	}
	if opts.addr != "127.0.0.1:7331" {
		t.Fatalf("addr = %q, want default", opts.addr)
	}
	if opts.path != "/mcp" {
		t.Fatalf("path = %q, want /mcp", opts.path)
	}
	if opts.allowedOrigins != nil {
		t.Fatalf("allowedOrigins = %#v, want nil", opts.allowedOrigins)
	}
	if opts.serverName != "ctx-mcp" {
		t.Fatalf("serverName = %q, want ctx-mcp", opts.serverName)
	}
}

func TestParseServeArgsHTTPOptions(t *testing.T) {
	opts, err := parseServeArgsWithSettings([]string{
		"--http",
		"--addr", "127.0.0.1:7444",
		"--path", "/ctx-mcp",
		"--name", "ctx-test",
		"--allowed-origins", "https://a.example, https://b.example",
	}, config.Settings{})
	if err != nil {
		t.Fatalf("parseServeArgs error: %v", err)
	}
	if !opts.httpMode {
		t.Fatal("httpMode = false, want true")
	}
	if opts.addr != "127.0.0.1:7444" {
		t.Fatalf("addr = %q, want custom", opts.addr)
	}
	if opts.path != "/ctx-mcp" {
		t.Fatalf("path = %q, want /ctx-mcp", opts.path)
	}
	if opts.serverName != "ctx-test" {
		t.Fatalf("serverName = %q, want ctx-test", opts.serverName)
	}
	want := []string{"https://a.example", "https://b.example"}
	if len(opts.allowedOrigins) != len(want) {
		t.Fatalf("allowedOrigins = %#v, want %#v", opts.allowedOrigins, want)
	}
	for i := range want {
		if opts.allowedOrigins[i] != want[i] {
			t.Fatalf("allowedOrigins[%d] = %q, want %q", i, opts.allowedOrigins[i], want[i])
		}
	}
}

func TestParseServeArgsUsesSettingsDefaults(t *testing.T) {
	opts, err := parseServeArgsWithSettings([]string{"--http"}, config.Settings{
		MCPHTTPAddr:          "127.0.0.1:7555",
		MCPHTTPPath:          "ctx",
		MCPServerName:        "ctx-work",
		MCPAllowedOrigins:    []string{"https://allowed.example"},
		MCPOAuthClientID:     "claude",
		MCPOAuthClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("parseServeArgs error: %v", err)
	}
	if opts.addr != "127.0.0.1:7555" {
		t.Fatalf("addr = %q, want settings value", opts.addr)
	}
	if opts.path != "/ctx" {
		t.Fatalf("path = %q, want normalized settings value", opts.path)
	}
	if opts.serverName != "ctx-work" {
		t.Fatalf("serverName = %q, want settings value", opts.serverName)
	}
	if len(opts.allowedOrigins) != 1 || opts.allowedOrigins[0] != "https://allowed.example" {
		t.Fatalf("allowedOrigins = %#v, want settings value", opts.allowedOrigins)
	}
	if opts.auth.ClientID != "claude" || opts.auth.ClientSecret != "secret" {
		t.Fatalf("auth = %#v, want settings credentials", opts.auth)
	}
}

func TestParseServeArgsDebugFlagAndEnv(t *testing.T) {
	opts, err := parseServeArgsWithSettings([]string{"--http", "--debug"}, config.Settings{})
	if err != nil {
		t.Fatalf("parseServeArgs error: %v", err)
	}
	if !opts.debug {
		t.Fatal("debug = false, want true from flag")
	}

	t.Setenv("CTX_MCP_DEBUG", "true")
	opts, err = parseServeArgsWithSettings([]string{"--http"}, config.Settings{})
	if err != nil {
		t.Fatalf("parseServeArgs error: %v", err)
	}
	if !opts.debug {
		t.Fatal("debug = false, want true from CTX_MCP_DEBUG")
	}
}

func TestParseServeArgsRejectsExtraArgs(t *testing.T) {
	if _, err := parseServeArgsWithSettings([]string{"extra"}, config.Settings{}); err == nil {
		t.Fatal("expected extra argument error")
	}
}

func TestParseServeArgsStdioFlag(t *testing.T) {
	opts, err := parseServeArgsWithSettings([]string{"--stdio"}, config.Settings{})
	if err != nil {
		t.Fatalf("parseServeArgs error: %v", err)
	}
	if !opts.stdioMode {
		t.Fatal("stdioMode = false, want true")
	}
	if opts.httpMode {
		t.Fatal("httpMode = true, want false")
	}
}

func TestParseServeArgsRejectsHTTPAndStdioTogether(t *testing.T) {
	if _, err := parseServeArgsWithSettings([]string{"--http", "--stdio"}, config.Settings{}); err == nil {
		t.Fatal("expected --http and --stdio together to error")
	}
}

func TestParseServeArgsNeitherFlagSetsNeitherMode(t *testing.T) {
	opts, err := parseServeArgsWithSettings(nil, config.Settings{})
	if err != nil {
		t.Fatalf("parseServeArgs error: %v", err)
	}
	if opts.httpMode || opts.stdioMode {
		t.Fatalf("httpMode=%v stdioMode=%v, want both false", opts.httpMode, opts.stdioMode)
	}
}
