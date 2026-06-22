package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(data))
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
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

func toolText(t *testing.T, resp map[string]any) string {
	t.Helper()
	result := resp["result"].(map[string]any)
	content := result["content"].([]any)
	first := content[0].(map[string]any)
	return first["text"].(string)
}
