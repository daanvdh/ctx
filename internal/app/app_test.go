package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ctx/internal/model"
)

type fakeStore struct {
	createdID string
	parentID  *string
	values    map[string]string
	resolved  map[string]string
	nodes     []model.SessionNode
	err       error
}

func (f *fakeStore) CreateSession(_ context.Context, id string, parentID *string) error {
	f.createdID = id
	f.parentID = parentID
	return f.err
}

func (f *fakeStore) SetValue(_ context.Context, sessionID, key, value string) error {
	if f.values == nil {
		f.values = make(map[string]string)
	}
	f.values[sessionID+"."+key] = value
	return f.err
}

func (f *fakeStore) GetValue(_ context.Context, _, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.values[key], nil
}

func (f *fakeStore) Resolve(context.Context, string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resolved, nil
}

func (f *fakeStore) SessionNodes(context.Context) ([]model.SessionNode, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.nodes, nil
}

func (f *fakeStore) DeleteSessionTree(context.Context, string) error {
	return f.err
}

func TestCreateSessionUsesInjectedStore(t *testing.T) {
	parent := "root"
	fake := &fakeStore{}
	a := NewWithStore(fake)

	id, err := a.CreateSession(context.Background(), "child", &parent)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if id != "child" || fake.createdID != "child" {
		t.Fatalf("created id = %q/%q, want child", id, fake.createdID)
	}
	if fake.parentID == nil || *fake.parentID != "root" {
		t.Fatalf("parent = %v, want root", fake.parentID)
	}
}

func TestExportRejectsInvalidShellKey(t *testing.T) {
	a := NewWithStore(&fakeStore{resolved: map[string]string{"1BAD": "value"}})
	_, err := a.Export(context.Background(), "s1")
	if err == nil {
		t.Fatal("expected invalid shell key error")
	}
}

func TestRenderUsesInjectedStore(t *testing.T) {
	a := NewWithStore(&fakeStore{
		values:   map[string]string{"PROMPT": "Fix $ISSUE"},
		resolved: map[string]string{"ISSUE": "22"},
	})

	got, err := a.Render(context.Background(), "s1", "PROMPT", false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if got != "Fix 22" {
		t.Fatalf("Render = %q, want Fix 22", got)
	}
}

func TestTreeJSONFormat(t *testing.T) {
	a := NewWithStore(&fakeStore{nodes: []model.SessionNode{{ID: "root", Data: map[string]string{"K": "V"}}}})
	got, err := a.Tree(context.Background(), TreeFormatJSON)
	if err != nil {
		t.Fatalf("Tree error: %v", err)
	}
	if !strings.Contains(got, `"id": "root"`) {
		t.Fatalf("json tree output = %s", got)
	}
}

func TestStoreErrorsPropagate(t *testing.T) {
	want := errors.New("boom")
	a := NewWithStore(&fakeStore{err: want})
	if err := a.SetValue(context.Background(), "s1", "K", "V"); !errors.Is(err, want) {
		t.Fatalf("SetValue error = %v, want %v", err, want)
	}
}
