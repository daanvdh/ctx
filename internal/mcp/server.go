package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ctx/internal/app"
	"ctx/internal/model"
)

const protocolVersion = "2025-06-18"

type Server struct {
	in     *bufio.Reader
	out    io.Writer
	newApp func() (*app.App, error)
	name   string
}

type HTTPOptions struct {
	AllowedOrigins []string
	ServerName     string
	Auth           *HTTPAuth
}

type AuthConfig struct {
	ClientID          string
	ClientSecret      string
	StaticBearerToken string
	PublicURL         string
	ResourcePath      string
	ServerName        string
}

type HTTPAuth struct {
	cfg    AuthConfig
	mu     sync.Mutex
	codes  map[string]authCode
	tokens map[string]time.Time
	now    func() time.Time
}

type authCode struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	ExpiresAt     time.Time
}

func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{
		in:     bufio.NewReader(in),
		out:    out,
		newApp: app.New,
		name:   "ctx-mcp",
	}
}

func NewServerWithApp(in io.Reader, out io.Writer, newApp func() (*app.App, error)) *Server {
	s := NewServer(in, out)
	s.newApp = newApp
	return s
}

func NewHTTPHandler(newApp func() (*app.App, error), allowedOrigins []string) http.Handler {
	return NewHTTPHandlerWithOptions(newApp, HTTPOptions{AllowedOrigins: allowedOrigins})
}

func NewHTTPHandlerWithOptions(newApp func() (*app.App, error), opts HTTPOptions) http.Handler {
	name := strings.TrimSpace(opts.ServerName)
	if name == "" {
		name = "ctx-mcp"
	}
	s := &Server{newApp: newApp, name: name}
	origins := make(map[string]struct{}, len(opts.AllowedOrigins))
	for _, origin := range opts.AllowedOrigins {
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
		if opts.Auth != nil && opts.Auth.Enabled() && !opts.Auth.AuthorizeMCP(w, r) {
			return
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

type accessLogResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *accessLogResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *accessLogResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func NewAccessLogHandler(next http.Handler, dst io.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rpc := ""
		if r.Body != nil && r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err == nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
				rpc = rpcLogSummary(body)
			}
		}
		lw := &accessLogResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lw, r)
		status := lw.status
		if status == 0 {
			status = http.StatusOK
		}
		auth := "no"
		if bearerToken(r.Header.Get("Authorization")) != "" {
			auth = "bearer"
		}
		rpcPart := ""
		if rpc != "" {
			rpcPart = " rpc=" + rpc
		}
		fmt.Fprintf(dst, "ctx: http: method=%s path=%s status=%d bytes=%d duration=%s auth=%s%s accept=%q origin=%q ua=%q\n",
			r.Method,
			r.URL.Path,
			status,
			lw.bytes,
			time.Since(start).Round(time.Millisecond),
			auth,
			rpcPart,
			r.Header.Get("Accept"),
			r.Header.Get("Origin"),
			r.UserAgent(),
		)
	})
}

func rpcLogSummary(body []byte) string {
	var req request
	if err := json.Unmarshal(body, &req); err != nil || req.Method == "" {
		return ""
	}
	out := req.Method
	if req.Method == "tools/call" {
		var call toolCall
		if err := json.Unmarshal(req.Params, &call); err == nil && call.Name != "" {
			out += ":" + call.Name
		}
	}
	return strconv.Quote(out)
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
				"name":    s.name,
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

func NewHTTPAuth(cfg AuthConfig) *HTTPAuth {
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)
	cfg.StaticBearerToken = strings.TrimSpace(cfg.StaticBearerToken)
	cfg.PublicURL = strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	cfg.ResourcePath = normalizePath(cfg.ResourcePath, "/mcp")
	cfg.ServerName = strings.TrimSpace(cfg.ServerName)
	if cfg.ServerName == "" {
		cfg.ServerName = "ctx-mcp"
	}
	return &HTTPAuth{
		cfg:    cfg,
		codes:  make(map[string]authCode),
		tokens: make(map[string]time.Time),
		now:    time.Now,
	}
}

func (a *HTTPAuth) Enabled() bool {
	return a != nil && (a.cfg.StaticBearerToken != "" || (a.cfg.ClientID != "" && a.cfg.ClientSecret != ""))
}

func (a *HTTPAuth) Register(mux *http.ServeMux) {
	mux.HandleFunc("/.well-known/oauth-protected-resource", a.handleProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource/", a.handleProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", a.handleAuthorizationServerMetadata)
	mux.HandleFunc("/oauth/authorize", a.handleAuthorize)
	mux.HandleFunc("/oauth/token", a.handleToken)
}

func (a *HTTPAuth) AuthorizeMCP(w http.ResponseWriter, r *http.Request) bool {
	token := bearerToken(r.Header.Get("Authorization"))
	if token != "" && a.validToken(token) {
		return true
	}
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, a.resourceMetadataURL(r)))
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (a *HTTPAuth) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	origin := a.origin(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 origin + a.cfg.ResourcePath,
		"authorization_servers":    []string{origin},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            a.cfg.ServerName,
		"scopes_supported":         []string{},
	})
}

func (a *HTTPAuth) handleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	origin := a.origin(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                origin,
		"authorization_endpoint":                origin + "/oauth/authorize",
		"token_endpoint":                        origin + "/oauth/token",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{},
	})
}

func (a *HTTPAuth) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	if !a.oauthClientConfigured() || q.Get("response_type") != "code" || q.Get("client_id") != a.cfg.ClientID {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	redirectURI := q.Get("redirect_uri")
	redirect, err := url.Parse(redirectURI)
	if err != nil || redirect.Scheme == "" || redirect.Host == "" {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		http.Error(w, "S256 PKCE is required", http.StatusBadRequest)
		return
	}

	code, err := randomURLToken(32)
	if err != nil {
		http.Error(w, "failed to create authorization code", http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.codes[code] = authCode{
		ClientID:      a.cfg.ClientID,
		RedirectURI:   redirectURI,
		CodeChallenge: q.Get("code_challenge"),
		ExpiresAt:     a.now().Add(5 * time.Minute),
	}
	a.mu.Unlock()

	out := redirect.Query()
	out.Set("code", code)
	if state := q.Get("state"); state != "" {
		out.Set("state", state)
	}
	redirect.RawQuery = out.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (a *HTTPAuth) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid token request", http.StatusBadRequest)
		return
	}
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		clientID = r.PostForm.Get("client_id")
		clientSecret = r.PostForm.Get("client_secret")
	}
	if !a.validClient(clientID, clientSecret) || r.PostForm.Get("grant_type") != "authorization_code" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	codeValue := r.PostForm.Get("code")
	a.mu.Lock()
	code, ok := a.codes[codeValue]
	if ok {
		delete(a.codes, codeValue)
	}
	a.mu.Unlock()
	if !ok || a.now().After(code.ExpiresAt) || code.ClientID != clientID || code.RedirectURI != r.PostForm.Get("redirect_uri") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	if !validPKCE(code.CodeChallenge, r.PostForm.Get("code_verifier")) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}

	token, err := randomURLToken(32)
	if err != nil {
		http.Error(w, "failed to create access token", http.StatusInternalServerError)
		return
	}
	expiresIn := 24 * time.Hour
	a.mu.Lock()
	a.tokens[token] = a.now().Add(expiresIn)
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(expiresIn.Seconds()),
	})
}

func (a *HTTPAuth) resourceMetadataURL(r *http.Request) string {
	return a.origin(r) + "/.well-known/oauth-protected-resource" + a.cfg.ResourcePath
}

func (a *HTTPAuth) origin(r *http.Request) string {
	if a.cfg.PublicURL != "" {
		return a.cfg.PublicURL
	}
	proto := firstForwarded(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := firstForwarded(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
}

func (a *HTTPAuth) oauthClientConfigured() bool {
	return a.cfg.ClientID != "" && a.cfg.ClientSecret != ""
}

func (a *HTTPAuth) validClient(clientID, clientSecret string) bool {
	return a.oauthClientConfigured() &&
		constantTimeEqual(clientID, a.cfg.ClientID) &&
		constantTimeEqual(clientSecret, a.cfg.ClientSecret)
}

func (a *HTTPAuth) validToken(token string) bool {
	if a.cfg.StaticBearerToken != "" && constantTimeEqual(token, a.cfg.StaticBearerToken) {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	expiresAt, ok := a.tokens[token]
	if !ok {
		return false
	}
	if a.now().After(expiresAt) {
		delete(a.tokens, token)
		return false
	}
	return true
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func firstForwarded(value string) string {
	if before, _, ok := strings.Cut(value, ","); ok {
		value = before
	}
	return strings.TrimSpace(value)
}

func normalizePath(path, fallback string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = fallback
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func validPKCE(challenge, verifier string) bool {
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	return constantTimeEqual(got, challenge)
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
		valueType := model.ValueTypeString
		if boolDefault(args, "is_doc", false) {
			valueType = model.ValueTypeDoc
		}
		err := a.SetEntry(ctx, requiredString(args, "session_id"), requiredString(args, "key"), model.NewEntry(requiredString(args, "value"), valueType))
		if err != nil {
			return toolError(err), nil
		}
		out = ok()
	case "ctx_get":
		var value string
		if boolDefault(args, "preview", false) {
			value, err = a.GetPreview(ctx, requiredString(args, "session_id"), requiredString(args, "key"))
		} else {
			value, err = a.GetValue(ctx, requiredString(args, "session_id"), requiredString(args, "key"))
		}
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
		entries, err := a.ShowEntries(ctx, requiredString(args, "session_id"))
		if err != nil {
			return toolError(err), nil
		}
		out = map[string]any{"lines": lines, "text": strings.Join(lines, "\n"), "entries": entries}
	case "ctx_export":
		lines, err := a.Export(ctx, requiredString(args, "session_id"), false, false, false)
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
		text, err := a.Tree(ctx, stringDefault(args, "format", app.TreeFormatText), stringDefault(args, "session_id", ""))
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
