package cmd

import "testing"

func TestParseServeArgsDefaultsToStdio(t *testing.T) {
	opts, err := parseServeArgs(nil)
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
}

func TestParseServeArgsHTTPOptions(t *testing.T) {
	opts, err := parseServeArgs([]string{
		"--http",
		"--addr", "127.0.0.1:7444",
		"--path", "/ctx-mcp",
		"--allowed-origins", "https://a.example, https://b.example",
	})
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

func TestParseServeArgsRejectsExtraArgs(t *testing.T) {
	if _, err := parseServeArgs([]string{"extra"}); err == nil {
		t.Fatal("expected extra argument error")
	}
}
