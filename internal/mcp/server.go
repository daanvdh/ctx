package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ctx/internal/app"
)

const protocolVersion = "2025-06-18"

type Server struct {
	in     *bufio.Reader
	out    io.Writer
	newApp func() (*app.App, error)
}

func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{
		in:     bufio.NewReader(in),
		out:    out,
		newApp: app.New,
	}
}

func NewServerWithApp(in io.Reader, out io.Writer, newApp func() (*app.App, error)) *Server {
	s := NewServer(in, out)
	s.newApp = newApp
	return s
}

func NewHTTPHandler(newApp func() (*app.App, error), allowedOrigins []string) http.Handler {
	s := &Server{newApp: newApp}
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins[origin] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			if _, ok := origins[origin]; !ok {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
		}

		switch r.Method {
		case http.MethodPost:
			if !accepts(r.Header.Get("Accept"), "application/json") || !accepts(r.Header.Get("Accept"), "text/event-stream") {
				http.Error(w, "Accept must include application/json and text/event-stream", http.StatusNotAcceptable)
				return
			}
			defer r.Body.Close()
			var req request
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, response{JSONRPC: "2.0", Error: rpcError(-32700, "parse error", err.Error())})
				return
			}
			if req.ID == nil {
				w.WriteHeader(http.StatusAccepted)
				return
			}

			result, callErr := s.handle(r.Context(), req)
			resp := response{JSONRPC: "2.0", ID: req.ID}
			if callErr != nil {
				resp.Error = rpcError(-32603, callErr.Error(), nil)
			} else {
				resp.Result = result
			}
			writeJSON(w, http.StatusOK, resp)
		case http.MethodGet:
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "SSE streams are not implemented by this ctx MCP POC", http.StatusMethodNotAllowed)
		default:
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) Serve(ctx context.Context) error {
	for {
		msg, err := readMessage(s.in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		var req request
		if err := json.Unmarshal(msg, &req); err != nil {
			_ = s.write(response{JSONRPC: "2.0", Error: rpcError(-32700, "parse error", err.Error())})
			continue
		}
		if req.ID == nil {
			continue
		}

		result, callErr := s.handle(ctx, req)
		resp := response{JSONRPC: "2.0", ID: req.ID}
		if callErr != nil {
			resp.Error = rpcError(-32603, callErr.Error(), nil)
		} else {
			resp.Result = result
		}
		if err := s.write(resp); err != nil {
			return err
		}
	}
}

func (s *Server) handle(ctx context.Context, req request) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "ctx-mcp",
				"version": "dev",
			},
		}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		return s.callTool(ctx, req.Params)
	default:
		return nil, fmt.Errorf("method not found: %s", req.Method)
	}
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if argErr, ok := recovered.(argumentError); ok {
				result = toolError(argErr)
				err = nil
				return
			}
			panic(recovered)
		}
	}()
	var call toolCall
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}
	a, err := s.newApp()
	if err != nil {
		return nil, err
	}

	args := call.Arguments
	var out any
	switch call.Name {
	case "ctx_new":
		var parent *string
		if p, ok := optionalString(args, "parent"); ok {
			parent = &p
		}
		id, err := a.CreateSession(ctx, stringArg(args, "id"), parent)
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]string{"id": id}
	case "ctx_set":
		err := a.SetValue(ctx, requiredString(args, "session_id"), requiredString(args, "key"), requiredString(args, "value"))
		if err != nil {
			return toolError(err), nil
		}
		out = ok()
	case "ctx_get":
		value, err := a.GetValue(ctx, requiredString(args, "session_id"), requiredString(args, "key"))
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]string{"value": value}
	case "ctx_resolve":
		values, err := a.Resolve(ctx, requiredString(args, "session_id"))
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]any{"values": values}
	case "ctx_show":
		lines, err := a.Show(ctx, requiredString(args, "session_id"))
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]any{"lines": lines, "text": strings.Join(lines, "\n")}
	case "ctx_export":
		lines, err := a.Export(ctx, requiredString(args, "session_id"))
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]any{"lines": lines, "text": strings.Join(lines, "\n")}
	case "ctx_share":
		err := a.ShareContext(ctx, requiredString(args, "from_session_id"), requiredString(args, "to_session_id"))
		if err != nil {
			return toolError(err), nil
		}
		out = ok()
	case "ctx_tree":
		text, err := a.Tree(ctx, stringDefault(args, "format", app.TreeFormatText))
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]string{"text": text}
	case "ctx_render":
		text, err := a.Render(ctx, requiredString(args, "session_id"), requiredString(args, "key"), boolDefault(args, "ignore_missing", false))
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]string{"text": text}
	case "ctx_delete":
		err := a.DeleteSessionTree(ctx, requiredString(args, "session_id"))
		if err != nil {
			return toolError(err), nil
		}
		out = ok()
	case "ctx_execute":
		err := a.Execute(ctx, requiredString(args, "session_id"), requiredString(args, "template"))
		if err != nil {
			return toolError(err), nil
		}
		out = ok()
	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Name)
	}

	return toolResult(out)
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	next, err := r.Peek(1)
	if err != nil {
		return nil, err
	}
	if next[0] == '{' {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && strings.TrimSpace(line) != "" {
				return []byte(strings.TrimSpace(line)), nil
			}
			return nil, err
		}
		return []byte(strings.TrimSpace(line)), nil
	}

	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header line %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Server) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.out, "%s\n", data)
	return err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func accepts(header, mediaType string) bool {
	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(strings.Split(part, ";")[0])
		if value == mediaType || value == "*/*" {
			return true
		}
	}
	return false
}

func toolResult(v any) (map[string]any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": string(data)}},
	}, nil
}

func toolError(err error) map[string]any {
	return map[string]any{
		"isError": true,
		"content": []map[string]string{{
			"type": "text",
			"text": err.Error(),
		}},
	}
}

func ok() map[string]bool {
	return map[string]bool{"ok": true}
}

func requiredString(args map[string]json.RawMessage, key string) string {
	value, ok := optionalString(args, key)
	if !ok || value == "" {
		panic(argumentError(fmt.Sprintf("missing required string argument %q", key)))
	}
	return value
}

func optionalString(args map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := args[key]
	if !ok || len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		panic(argumentError(fmt.Sprintf("argument %q must be a string", key)))
	}
	return value, true
}

func stringArg(args map[string]json.RawMessage, key string) string {
	value, _ := optionalString(args, key)
	return value
}

func stringDefault(args map[string]json.RawMessage, key, fallback string) string {
	if value, ok := optionalString(args, key); ok {
		return value
	}
	return fallback
}

func boolDefault(args map[string]json.RawMessage, key string, fallback bool) bool {
	raw, ok := args[key]
	if !ok || len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return fallback
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		panic(argumentError(fmt.Sprintf("argument %q must be a boolean", key)))
	}
	return value
}

type argumentError string

func (e argumentError) Error() string {
	return string(e)
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type toolCall struct {
	Name      string                     `json:"name"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

func rpcError(code int, message string, data any) map[string]any {
	err := map[string]any{"code": code, "message": message}
	if data != nil {
		err["data"] = data
	}
	return err
}
