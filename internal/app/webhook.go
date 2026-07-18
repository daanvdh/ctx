package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ctx/internal/config"
	"ctx/internal/model"
	"ctx/internal/store"
	"gopkg.in/yaml.v3"
)

// webhookFieldSpec locates a value in a delivery: an HTTP header or a
// dotted JSON path into the payload. Exactly one should be set.
type webhookFieldSpec struct {
	Header string `yaml:"header"`
	Field  string `yaml:"field"`
}

type webhookVerify struct {
	Type      string `yaml:"type"`
	Header    string `yaml:"header"`
	SecretEnv string `yaml:"secret_env"`
}

// webhookAdapter is the per-source config file at
// ~/.config/ctx/webhooks/<source>.yaml. Adding a source is config-only:
// drop a file there and POST /webhooks/<source> starts working.
type webhookAdapter struct {
	Verify     webhookVerify     `yaml:"verify"`
	EventType  webhookFieldSpec  `yaml:"event_type"`
	DeliveryID webhookFieldSpec  `yaml:"delivery_id"`
	Fields     map[string]string `yaml:"fields"`
	// Session receives the WEBHOOK_* entries; defaults to the source name.
	Session string `yaml:"session"`
}

func loadWebhookAdapter(source string) (webhookAdapter, error) {
	dir, err := config.WebhookDir()
	if err != nil {
		return webhookAdapter{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, source+".yaml"))
	if err != nil {
		return webhookAdapter{}, err
	}
	var adapter webhookAdapter
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&adapter); err != nil {
		return webhookAdapter{}, fmt.Errorf("malformed webhook adapter %s: %w", source, err)
	}
	return adapter, nil
}

// check verifies a delivery against the adapter's auth config. Any failure
// (including a missing secret) fails closed.
func (v webhookVerify) check(r *http.Request, body []byte) error {
	if v.Type == "none" {
		return nil
	}
	secret := os.Getenv(v.SecretEnv)
	if secret == "" {
		return fmt.Errorf("secret env %s is unset", v.SecretEnv)
	}
	switch v.Type {
	case "hmac_sha256":
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(want), []byte(r.Header.Get(v.Header))) {
			return nil
		}
		return fmt.Errorf("signature mismatch on header %s", v.Header)
	case "shared_token":
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(v.Header)), []byte(secret)) == 1 {
			return nil
		}
		return fmt.Errorf("token mismatch on header %s", v.Header)
	case "bearer":
		header := v.Header
		if header == "" {
			header = "Authorization"
		}
		got := strings.TrimPrefix(r.Header.Get(header), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1 {
			return nil
		}
		return fmt.Errorf("bearer token mismatch on header %s", header)
	default:
		return fmt.Errorf("unknown verify type %q", v.Type)
	}
}

func (s webhookFieldSpec) extract(r *http.Request, payload map[string]any) string {
	if s.Header != "" {
		return r.Header.Get(s.Header)
	}
	if s.Field != "" {
		return jsonPath(payload, s.Field)
	}
	return ""
}

// jsonPath resolves a dotted path like "repository.full_name" against a
// decoded JSON object and renders the leaf as a string.
// ponytail: objects only, no array indexing; add when an adapter needs it.
func jsonPath(payload map[string]any, path string) string {
	var current any = payload
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		if current, ok = obj[part]; !ok {
			return ""
		}
	}
	switch v := current.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}

// webhookHandler serves POST /webhooks/{source}: verify per adapter config,
// dedupe on (source, delivery id), write the normalized event as WEBHOOK_*
// entries into the adapter's session, and fire matching triggers on the
// final WEBHOOK_EVENT write. Trigger files match events with the existing
// entries filters (WEBHOOK_EVENT, WEBHOOK_REPO, ...) — no separate lookup
// table.
type webhookHandler struct {
	app *App
	mu  sync.Mutex
	// seen dedupes deliveries in memory. ponytail: lost on restart; a
	// schedule trigger doubles as the reconciliation poll for anything
	// missed, so persistence buys little.
	seen map[string]bool
}

// WebhookHandler returns the HTTP handler for /webhooks/{source}.
func (a *App) WebhookHandler() http.Handler {
	return &webhookHandler{app: a, seen: make(map[string]bool)}
}

func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	source := strings.TrimPrefix(r.URL.Path, "/webhooks/")
	if source == "" || strings.ContainsAny(source, "/\\.") {
		http.Error(w, "invalid webhook source", http.StatusNotFound)
		return
	}
	adapter, err := loadWebhookAdapter(source)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "unknown webhook source", http.StatusNotFound)
			return
		}
		fmt.Fprintf(h.app.stderr, "ctx: webhook %s: %v\n", source, err)
		http.Error(w, "webhook adapter error", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(maxStringBytes())))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := adapter.Verify.check(r, body); err != nil {
		fmt.Fprintf(h.app.stderr, "ctx: webhook %s: rejected: %v\n", source, err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload map[string]any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		needsPayload := adapter.EventType.Field != "" || adapter.DeliveryID.Field != "" || len(adapter.Fields) > 0
		if needsPayload {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	eventType := adapter.EventType.extract(r, payload)
	if eventType == "" {
		http.Error(w, "missing event type", http.StatusBadRequest)
		return
	}
	deliveryID := adapter.DeliveryID.extract(r, payload)

	dedupeKey := source + "\n" + deliveryID
	if deliveryID != "" {
		h.mu.Lock()
		if h.seen[dedupeKey] {
			h.mu.Unlock()
			fmt.Fprintln(w, "duplicate delivery ignored")
			return
		}
		if len(h.seen) >= 10000 { // ponytail: crude cap; LRU if it matters
			h.seen = make(map[string]bool)
		}
		h.seen[dedupeKey] = true
		h.mu.Unlock()
	}

	if err := h.store(r.Context(), adapter, source, eventType, deliveryID, body, payload); err != nil {
		if deliveryID != "" {
			h.mu.Lock()
			delete(h.seen, dedupeKey)
			h.mu.Unlock()
		}
		fmt.Fprintf(h.app.stderr, "ctx: webhook %s: %v\n", source, err)
		http.Error(w, "failed to store event", http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "ok")
}

// store writes the normalized event into the adapter's session and fires
// triggers for the final WEBHOOK_EVENT write. Earlier writes go through the
// store directly so triggers only ever observe a fully written event.
func (h *webhookHandler) store(ctx context.Context, adapter webhookAdapter, source, eventType, deliveryID string, body []byte, payload map[string]any) error {
	a := h.app
	sessionID := adapter.Session
	if sessionID == "" {
		sessionID = source
	}
	if _, err := a.CreateSession(ctx, sessionID, nil, true); err != nil && !store.IsAlreadyExists(err) {
		return fmt.Errorf("create session %s: %w", sessionID, err)
	}

	entries := map[string]string{
		"WEBHOOK_SOURCE":   source,
		"WEBHOOK_DELIVERY": deliveryID,
		"WEBHOOK_PAYLOAD":  string(body),
	}
	for name, path := range adapter.Fields {
		entries["WEBHOOK_"+shellSafeIdentifier(strings.ToUpper(name))] = jsonPath(payload, path)
	}
	for key, value := range entries {
		if err := a.store.SetEntry(ctx, sessionID, key, model.NewEntry(value, model.ValueTypeString)); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}

	oldValue, err := a.store.GetValue(ctx, sessionID, "WEBHOOK_EVENT")
	if err != nil {
		oldValue = ""
	}
	if err := a.store.SetEntry(ctx, sessionID, "WEBHOOK_EVENT", model.NewEntry(eventType, model.ValueTypeString)); err != nil {
		return fmt.Errorf("set WEBHOOK_EVENT: %w", err)
	}
	change := TriggerChange{SessionID: sessionID, Key: "WEBHOOK_EVENT", OldValue: oldValue, NewValue: eventType}
	// Fire in the background so slow trigger scripts don't hit the
	// sender's delivery timeout.
	go func() {
		if err := a.ExecuteMatchingTriggers(context.Background(), change); err != nil {
			fmt.Fprintf(a.stderr, "ctx: webhook %s: triggers: %v\n", source, err)
		}
	}()
	return nil
}
