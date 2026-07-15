package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ctx/internal/model"
)

// RemoteStore implements Store by calling a remote ctx MCP server's
// tools/call endpoint over Streamable HTTP, so a client can use that server
// as its backend instead of a local sqlite db.
type RemoteStore struct {
	url    string
	token  string
	client *http.Client
}

func NewRemote(url, token string) *RemoteStore {
	return &RemoteStore{url: url, token: token, client: &http.Client{Timeout: 30 * time.Second}}
}

type remoteRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type remoteToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type remoteRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type remoteToolResult struct {
	IsError bool `json:"isError"`
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// call invokes a single MCP tool and returns its raw JSON text content.
// It sends one request per call, with no initialize handshake: the server
// dispatches tools/call statelessly, so a handshake would be a wasted round
// trip.
func (s *RemoteStore) call(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(remoteRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  remoteToolCallParams{Name: name, Arguments: args},
	})
	if err != nil {
		return nil, fmt.Errorf("remote mcp: %s: encode request: %w", name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("remote mcp: %s: build request: %w", name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote mcp: %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote mcp: %s: unexpected status %d: %s", name, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var rpcResp remoteRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("remote mcp: %s: decode response (status %d): %w", name, resp.StatusCode, err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("remote mcp: %s: %s", name, rpcResp.Error.Message)
	}

	var result remoteToolResult
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		return nil, fmt.Errorf("remote mcp: %s: decode result: %w", name, err)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("remote mcp: %s: empty response", name)
	}
	if result.IsError {
		return nil, fmt.Errorf("remote mcp: %s: %s", name, result.Content[0].Text)
	}
	return json.RawMessage(result.Content[0].Text), nil
}

func (s *RemoteStore) CreateSession(ctx context.Context, id string, parentID *string) error {
	args := map[string]any{"id": id}
	if parentID != nil {
		args["parent"] = *parentID
	}
	_, err := s.call(ctx, "ctx_new", args)
	if err != nil {
		// ponytail: matched by message text, since errors don't survive the
		// wire as typed values. A structured error code in the MCP protocol
		// would make this robust instead of string-matching.
		if strings.Contains(err.Error(), ErrAlreadyExists.Error()) {
			return fmt.Errorf("%w: %s", ErrAlreadyExists, id)
		}
		return err
	}
	return nil
}

func (s *RemoteStore) SetValue(ctx context.Context, sessionID, key, value string) error {
	return s.SetEntry(ctx, sessionID, key, model.NewEntry(value, model.ValueTypeString))
}

func (s *RemoteStore) SetEntry(ctx context.Context, sessionID, key string, entry model.Entry) error {
	switch entry.ValueType {
	case model.ValueTypeString:
		_, err := s.call(ctx, "ctx_set", map[string]any{
			"session_id": sessionID,
			"key":        key,
			"value":      entry.Value,
		})
		return err
	default:
		// ponytail: the ctx_set MCP tool has no file_ref/file_bin support yet
		// (even for a local server); add that there first, then wire it
		// through here.
		return fmt.Errorf("remote backend: value type %q is not supported over the MCP protocol yet", entry.ValueType)
	}
}

func (s *RemoteStore) RemoveEntry(ctx context.Context, sessionID, key string) error {
	// ponytail: no ctx_rm MCP tool exists yet; add one in
	// internal/mcp/tools.go and internal/mcp/server.go, then implement this
	// like SetEntry above.
	return fmt.Errorf("remote backend: rm is not supported yet (no ctx_rm MCP tool)")
}

func (s *RemoteStore) GetValue(ctx context.Context, sessionID, key string) (string, error) {
	raw, err := s.call(ctx, "ctx_get", map[string]any{"session_id": sessionID, "key": key})
	if err != nil {
		return "", err
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("remote mcp: ctx_get: decode: %w", err)
	}
	return out.Value, nil
}

func (s *RemoteStore) GetEntry(ctx context.Context, sessionID, key string) (model.Entry, error) {
	entries, err := s.ResolveEntries(ctx, sessionID)
	if err != nil {
		return model.Entry{}, err
	}
	entry, ok := entries[key]
	if !ok {
		return model.Entry{}, fmt.Errorf("key %s not found in session %s or ancestors", key, sessionID)
	}
	return entry, nil
}

func (s *RemoteStore) Resolve(ctx context.Context, sessionID string) (map[string]string, error) {
	raw, err := s.call(ctx, "ctx_resolve", map[string]any{"session_id": sessionID})
	if err != nil {
		return nil, err
	}
	var out struct {
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("remote mcp: ctx_resolve: decode: %w", err)
	}
	return out.Values, nil
}

func (s *RemoteStore) ResolveEntries(ctx context.Context, sessionID string) (map[string]model.Entry, error) {
	raw, err := s.call(ctx, "ctx_resolve_entries", map[string]any{"session_id": sessionID})
	if err != nil {
		return nil, err
	}
	var out struct {
		Entries map[string]model.Entry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("remote mcp: ctx_resolve_entries: decode: %w", err)
	}
	return out.Entries, nil
}

func (s *RemoteStore) ShareContext(ctx context.Context, fromSessionID, toSessionID string) error {
	_, err := s.call(ctx, "ctx_share", map[string]any{"from_session_id": fromSessionID, "to_session_id": toSessionID})
	return err
}

func (s *RemoteStore) SessionNodes(ctx context.Context) ([]model.SessionNode, error) {
	raw, err := s.call(ctx, "ctx_tree", map[string]any{"format": "json"})
	if err != nil {
		return nil, err
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("remote mcp: ctx_tree: decode: %w", err)
	}
	var nodes []model.SessionNode
	if err := json.Unmarshal([]byte(out.Text), &nodes); err != nil {
		return nil, fmt.Errorf("remote mcp: ctx_tree: decode nodes: %w", err)
	}
	return nodes, nil
}

func (s *RemoteStore) DeleteSession(ctx context.Context, sessionID string, recursive, noVar, noChild bool) error {
	_, err := s.call(ctx, "ctx_delete", map[string]any{
		"session_id": sessionID,
		"recursive":  recursive,
		"no_var":     noVar,
		"no_child":   noChild,
	})
	return err
}
