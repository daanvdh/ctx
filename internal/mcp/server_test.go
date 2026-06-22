package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ctx/internal/app"
)

func TestServerInitializeAndToolCalls(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var input bytes.Buffer
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "ctx_new",
			"arguments": map[string]any{"id": "root"},
		},
	})
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "ctx_set",
			"arguments": map[string]any{
				"session_id": "root",
				"key":        "PROJECT",
				"value":      "ctx",
			},
		},
	})
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "ctx_get",
			"arguments": map[string]any{
				"session_id": "root",
				"key":        "PROJECT",
			},
		},
	})

	var output bytes.Buffer
	if err := NewServer(&input, &output).Serve(context.Background()); err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	responses := readTestResponses(t, output.Bytes())
	if len(responses) != 5 {
		t.Fatalf("response count = %d, want 5", len(responses))
	}
	if got := responses[0]["result"].(map[string]any)["protocolVersion"]; got != protocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", got, protocolVersion)
	}
	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 11 {
		t.Fatalf("tools count = %d, want full ctx API", len(tools))
	}
	text := toolText(t, responses[4])
	if !strings.Contains(text, `"value": "ctx"`) {
		t.Fatalf("ctx_get response text = %s, want value", text)
	}
}

func TestServerReturnsToolErrorForMissingArgument(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var input bytes.Buffer
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "ctx_get",
			"arguments": map[string]any{"session_id": "root"},
		},
	})

	var output bytes.Buffer
	if err := NewServer(&input, &output).Serve(context.Background()); err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	resp := readTestResponses(t, output.Bytes())[0]
	result := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("isError = %v, want true", result["isError"])
	}
	if text := toolText(t, resp); !strings.Contains(text, "missing required string argument") {
		t.Fatalf("error text = %s", text)
	}
}

func TestHTTPHandlerServesStreamableHTTP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	handler := NewHTTPHandler(app.New, nil)

	resp := postMCP(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	result := body["result"].(map[string]any)
	if result["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", result["protocolVersion"], protocolVersion)
	}

	resp = postMCP(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "ctx_new",
			"arguments": map[string]any{"id": "http-root"},
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("tool status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
}

func TestHTTPHandlerRejectsUnexpectedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Origin", "https://evil.example")
	resp := httptest.NewRecorder()

	NewHTTPHandler(app.New, []string{"https://allowed.example"}).ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.Code)
	}
}

func TestHTTPHandlerRequiresBearerTokenWhenConfigured(t *testing.T) {
	auth := NewHTTPAuth(AuthConfig{
		StaticBearerToken: "secret-token",
		ResourcePath:      "/mcp",
		PublicURL:         "https://ctx.example",
	})
	handler := NewHTTPHandlerWithOptions(app.New, HTTPOptions{Auth: auth})

	resp := postMCP(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.Code)
	}
	if got := resp.Header().Get("WWW-Authenticate"); !strings.Contains(got, `resource_metadata="https://ctx.example/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("WWW-Authenticate = %q, want resource metadata", got)
	}

	req := newMCPRequest(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	req.Header.Set("Authorization", "Bearer secret-token")
	okResp := httptest.NewRecorder()
	handler.ServeHTTP(okResp, req)
	if okResp.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want 200: %s", okResp.Code, okResp.Body.String())
	}
}

func TestHTTPAuthAuthorizationCodeFlowIssuesUsableBearerToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	auth := NewHTTPAuth(AuthConfig{
		ClientID:     "claude",
		ClientSecret: "client-secret",
		ResourcePath: "/mcp",
		ServerName:   "ctx-test",
	})
	mux := http.NewServeMux()
	auth.Register(mux)
	mux.Handle("/mcp", NewHTTPHandlerWithOptions(app.New, HTTPOptions{Auth: auth, ServerName: "ctx-test"}))

	metaReq := httptest.NewRequest(http.MethodGet, "https://ctx.example/.well-known/oauth-protected-resource/mcp", nil)
	metaResp := httptest.NewRecorder()
	mux.ServeHTTP(metaResp, metaReq)
	if metaResp.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, want 200: %s", metaResp.Code, metaResp.Body.String())
	}
	var metaBody map[string]any
	if err := json.Unmarshal(metaResp.Body.Bytes(), &metaBody); err != nil {
		t.Fatalf("unmarshal metadata response: %v", err)
	}
	if _, ok := metaBody["scopes_supported"].([]any); !ok {
		t.Fatalf("scopes_supported = %#v, want array", metaBody["scopes_supported"])
	}

	verifier := "test-verifier"
	challenge := testPKCEChallenge(verifier)
	redirectURI := "https://claude.example/callback"
	authorizeURL := "/oauth/authorize?response_type=code&client_id=claude&redirect_uri=" + url.QueryEscape(redirectURI) + "&code_challenge_method=S256&code_challenge=" + url.QueryEscape(challenge) + "&state=abc"
	authReq := httptest.NewRequest(http.MethodGet, "https://ctx.example"+authorizeURL, nil)
	authResp := httptest.NewRecorder()
	mux.ServeHTTP(authResp, authReq)
	if authResp.Code != http.StatusFound {
		t.Fatalf("authorize status = %d, want 302: %s", authResp.Code, authResp.Body.String())
	}
	location, err := url.Parse(authResp.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	code := location.Query().Get("code")
	if code == "" {
		t.Fatalf("redirect location = %q, want code", location.String())
	}
	if location.Query().Get("state") != "abc" {
		t.Fatalf("state = %q, want abc", location.Query().Get("state"))
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	tokenReq := httptest.NewRequest(http.MethodPost, "https://ctx.example/oauth/token", strings.NewReader(form.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.SetBasicAuth("claude", "client-secret")
	tokenResp := httptest.NewRecorder()
	mux.ServeHTTP(tokenResp, tokenReq)
	if tokenResp.Code != http.StatusOK {
		t.Fatalf("token status = %d, want 200: %s", tokenResp.Code, tokenResp.Body.String())
	}
	var tokenBody map[string]any
	if err := json.Unmarshal(tokenResp.Body.Bytes(), &tokenBody); err != nil {
		t.Fatalf("unmarshal token response: %v", err)
	}
	token, ok := tokenBody["access_token"].(string)
	if !ok || token == "" {
		t.Fatalf("access_token = %#v, want non-empty string", tokenBody["access_token"])
	}

	req := newMCPRequest(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	mcpResp := httptest.NewRecorder()
	mux.ServeHTTP(mcpResp, req)
	if mcpResp.Code != http.StatusOK {
		t.Fatalf("MCP status = %d, want 200: %s", mcpResp.Code, mcpResp.Body.String())
	}
}

func TestAccessLogHandlerLogsRequestWithoutToken(t *testing.T) {
	var log bytes.Buffer
	handler := NewAccessLogHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}), &log)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ctx_get","arguments":{"session_id":"secret-session"}}}`))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	text := log.String()
	for _, want := range []string{"method=POST", "path=/mcp", "status=202", "auth=bearer", `rpc="tools/call:ctx_get"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("log = %q, want substring %q", text, want)
		}
	}
	for _, secret := range []string{"secret-token", "secret-session"} {
		if strings.Contains(text, secret) {
			t.Fatalf("log leaked secret %q: %q", secret, text)
		}
	}
}

func writeTestMessage(t *testing.T, dst *bytes.Buffer, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(dst, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

func postMCP(t *testing.T, handler http.Handler, v any) *httptest.ResponseRecorder {
	t.Helper()
	req := newMCPRequest(t, v)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func newMCPRequest(t *testing.T, v any) *http.Request {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(data))
	req.Header.Set("Accept", "application/json, text/event-stream")
	return req
}

func readTestResponses(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	src := bytes.NewReader(data)
	reader := bufio.NewReader(src)
	var responses []map[string]any
	for src.Len() > 0 || reader.Buffered() > 0 {
		msg, err := readMessage(reader)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		var resp map[string]any
		if err := json.Unmarshal(msg, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		responses = append(responses, resp)
	}
	return responses
}

func testPKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func toolText(t *testing.T, resp map[string]any) string {
	t.Helper()
	result := resp["result"].(map[string]any)
	content := result["content"].([]any)
	first := content[0].(map[string]any)
	return first["text"].(string)
}
