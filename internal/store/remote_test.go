package store_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"ctx/internal/app"
	"ctx/internal/mcp"
	"ctx/internal/model"
	"ctx/internal/store"
)

func TestRemoteStoreRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CTX_ID", "")
	srv := httptest.NewServer(mcp.NewHTTPHandler(app.New, nil))
	t.Cleanup(srv.Close)

	remote := store.NewRemote(srv.URL, "")
	ctx := context.Background()

	if err := remote.CreateSession(ctx, "root", nil); err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if err := remote.CreateSession(ctx, "root", nil); !store.IsAlreadyExists(err) {
		t.Fatalf("CreateSession duplicate = %v, want IsAlreadyExists", err)
	}
	parent := "root"
	if err := remote.CreateSession(ctx, "child", &parent); err != nil {
		t.Fatalf("CreateSession child error: %v", err)
	}

	if err := remote.SetValue(ctx, "root", "NAME", "ada"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}
	if err := remote.SetEntry(ctx, "root", "STORY", model.NewEntry("full doc content here", model.ValueTypeDoc)); err != nil {
		t.Fatalf("SetEntry doc error: %v", err)
	}

	value, err := remote.GetValue(ctx, "child", "NAME")
	if err != nil || value != "ada" {
		t.Fatalf("GetValue = %q, %v, want ada, nil", value, err)
	}

	entry, err := remote.GetEntry(ctx, "child", "STORY")
	if err != nil || entry.Value != "full doc content here" || entry.ValueType != model.ValueTypeDoc {
		t.Fatalf("GetEntry = %+v, %v, want full doc value and type", entry, err)
	}

	resolved, err := remote.Resolve(ctx, "child")
	if err != nil || resolved["NAME"] != "ada" {
		t.Fatalf("Resolve = %v, %v, want NAME=ada", resolved, err)
	}

	entries, err := remote.ResolveEntries(ctx, "child")
	if err != nil || entries["STORY"].ValueType != model.ValueTypeDoc {
		t.Fatalf("ResolveEntries = %v, %v, want STORY doc", entries, err)
	}

	if err := remote.ShareContext(ctx, "root", "does-not-exist"); err == nil {
		t.Fatal("expected ShareContext with nonexistent session to fail")
	}

	nodes, err := remote.SessionNodes(ctx)
	if err != nil || len(nodes) != 2 {
		t.Fatalf("SessionNodes = %v, %v, want 2 nodes", nodes, err)
	}

	if err := remote.DeleteSessionTree(ctx, "child"); err != nil {
		t.Fatalf("DeleteSessionTree error: %v", err)
	}
	if _, err := remote.GetValue(ctx, "child", "NAME"); err == nil {
		t.Fatal("expected GetValue on deleted session to fail")
	}

	if err := remote.RemoveEntry(ctx, "root", "NAME"); err == nil {
		t.Fatal("expected RemoveEntry to be unsupported over the remote backend")
	}
	if err := remote.SetEntry(ctx, "root", "FILE", model.NewEntry("/tmp/x", model.ValueTypeFileRef)); err == nil {
		t.Fatal("expected file_ref SetEntry to be unsupported over the remote backend")
	}
}

func TestRemoteStoreRequiresBearerTokenWhenConfigured(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CTX_ID", "")
	auth := mcp.NewHTTPAuth(mcp.AuthConfig{StaticBearerToken: "secret"})
	srv := httptest.NewServer(mcp.NewHTTPHandlerWithOptions(app.New, mcp.HTTPOptions{Auth: auth}))
	t.Cleanup(srv.Close)

	unauthenticated := store.NewRemote(srv.URL, "")
	if err := unauthenticated.CreateSession(context.Background(), "root", nil); err == nil {
		t.Fatal("expected request without bearer token to be rejected")
	}

	authenticated := store.NewRemote(srv.URL, "secret")
	if err := authenticated.CreateSession(context.Background(), "root", nil); err != nil {
		t.Fatalf("CreateSession with valid token error: %v", err)
	}
}
