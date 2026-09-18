package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWebhookAdapter(t *testing.T, source, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "ctx", "webhooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir webhooks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, source+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
}

const githubAdapter = `verify: { type: hmac_sha256, header: X-Hub-Signature-256, secret_env: GITHUB_WEBHOOK_SECRET }
event_type: { header: X-GitHub-Event }
delivery_id: { header: X-GitHub-Delivery }
fields: { repo: repository.full_name, ref_id: issue.number }
`

func TestWebhookGitHubDeliveryAndDedupe(t *testing.T) {
	writeWebhookAdapter(t, "github", githubAdapter)
	t.Setenv("GITHUB_WEBHOOK_SECRET", "s3cret")

	fake := &fakeStore{}
	handler := NewWithStore(fake).WebhookHandler()

	body := `{"repository":{"full_name":"daanvdh/ctx"},"issue":{"number":42}}`
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/webhooks/github", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "issues")
		req.Header.Set("X-GitHub-Delivery", "d-1")
		req.Header.Set("X-Hub-Signature-256", sig)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := send()
	if rec.Code != 200 {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	want := map[string]string{
		"github.WEBHOOK_EVENT":    "issues",
		"github.WEBHOOK_SOURCE":   "github",
		"github.WEBHOOK_DELIVERY": "d-1",
		"github.WEBHOOK_REPO":     "daanvdh/ctx",
		"github.WEBHOOK_REF_ID":   "42",
		"github.WEBHOOK_PAYLOAD":  body,
	}
	for key, value := range want {
		if got := fake.values[key]; got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}

	rec = send()
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "duplicate") {
		t.Fatalf("duplicate delivery: status = %d, body %q", rec.Code, rec.Body.String())
	}
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	writeWebhookAdapter(t, "github", githubAdapter)
	t.Setenv("GITHUB_WEBHOOK_SECRET", "s3cret")

	fake := &fakeStore{}
	handler := NewWithStore(fake).WebhookHandler()

	req := httptest.NewRequest("POST", "/webhooks/github", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "d-2")
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(fake.values) != 0 {
		t.Fatalf("rejected delivery was persisted: %v", fake.values)
	}
}

func TestWebhookUnknownSource(t *testing.T) {
	writeWebhookAdapter(t, "github", githubAdapter)

	handler := NewWithStore(&fakeStore{}).WebhookHandler()
	req := httptest.NewRequest("POST", "/webhooks/nope", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestWebhookSharedTokenAndFieldExtraction(t *testing.T) {
	writeWebhookAdapter(t, "jira", `verify: { type: bearer, secret_env: JIRA_WEBHOOK_TOKEN }
event_type: { field: webhookEvent }
delivery_id: { field: timestamp }
fields: { repo: issue.fields.project.key, ref_id: issue.key }
session: jira-events
`)
	t.Setenv("JIRA_WEBHOOK_TOKEN", "tok")

	fake := &fakeStore{}
	handler := NewWithStore(fake).WebhookHandler()

	body := `{"webhookEvent":"jira:issue_updated","timestamp":1721000000,"issue":{"key":"CTX-7","fields":{"project":{"key":"CTX"}}}}`
	req := httptest.NewRequest("POST", "/webhooks/jira", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	want := map[string]string{
		"jira-events.WEBHOOK_EVENT":    "jira:issue_updated",
		"jira-events.WEBHOOK_DELIVERY": "1721000000",
		"jira-events.WEBHOOK_REPO":     "CTX",
		"jira-events.WEBHOOK_REF_ID":   "CTX-7",
	}
	for key, value := range want {
		if got := fake.values[key]; got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
}
