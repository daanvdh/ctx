package session

import (
	"testing"

	"ctx/internal/model"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		parent  *string
		wantErr bool
	}{
		{
			name:    "root session",
			parent:  nil,
			wantErr: false,
		},
		{
			name:    "child session",
			parent:  func() *string { s := "parent-1"; return &s }(),
			wantErr: false,
		},
		{
			name:    "child of missing parent",
			parent:  func() *string { s := "nonexistent"; return &s }(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cf := &model.ContextFile{
				Sessions: make(map[string]*model.Session),
			}

			if tt.parent != nil && tt.name == "child session" {
				pID := "parent-1"
				cf.Sessions[pID] = &model.Session{
					Parent: nil,
					Data:   make(map[string]string),
				}
			}

			id, err := New(cf, tt.parent)
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if id == "" {
					t.Error("New() returned empty ID")
				}
				if _, ok := cf.Sessions[id]; !ok {
					t.Error("New() did not insert session into cf.Sessions")
				}
			}
		})
	}
}

func TestGet(t *testing.T) {
	cf := &model.ContextFile{
		Sessions: make(map[string]*model.Session),
	}

	parentID := "parent"
	cf.Sessions[parentID] = &model.Session{
		Parent: nil,
		Data:   map[string]string{"inherited": "from-parent"},
	}

	childID := "child"
	chParent := parentID
	cf.Sessions[childID] = &model.Session{
		Parent: &chParent,
		Data:   map[string]string{"own": "value"},
	}

	t.Run("get own key", func(t *testing.T) {
		val, err := Get(cf, childID, "own")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if val != "value" {
			t.Errorf("Get() = %q, want %q", val, "value")
		}
	})

	t.Run("get inherited key", func(t *testing.T) {
		val, err := Get(cf, childID, "inherited")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if val != "from-parent" {
			t.Errorf("Get() = %q, want %q", val, "from-parent")
		}
	})

	t.Run("get grandparent key", func(t *testing.T) {
		grandparentID := "grandparent"
		grandparentSession := &model.Session{
			Parent: nil,
			Data:   map[string]string{"from-grandparent": "yes"},
		}
		cf.Sessions[grandparentID] = grandparentSession
		parentSession := cf.Sessions[parentID]
		parentSession.Parent = &grandparentID

		val, err := Get(cf, childID, "from-grandparent")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if val != "yes" {
			t.Errorf("Get() = %q, want %q", val, "yes")
		}
	})

	t.Run("get missing key", func(t *testing.T) {
		_, err := Get(cf, childID, "missing")
		if err == nil {
			t.Error("Get() expected error for missing key, got nil")
		}
	})
}

func TestScopeShadowing(t *testing.T) {
	cf := &model.ContextFile{
		Sessions: make(map[string]*model.Session),
	}

	parentID := "parent"
	cf.Sessions[parentID] = &model.Session{
		Parent: nil,
		Data:   map[string]string{"key": "parent-value"},
	}

	childID := "child"
	chParent := parentID
	cf.Sessions[childID] = &model.Session{
		Parent: &chParent,
		Data:   map[string]string{"key": "child-value"},
	}

	val, err := Get(cf, childID, "key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if val != "child-value" {
		t.Errorf("Get() = %q, want %q (shadowing)", val, "child-value")
	}
}

func TestDepthCapCircular(t *testing.T) {
	cf := &model.ContextFile{
		Sessions: make(map[string]*model.Session),
	}

	a := "a"
	b := "b"
	chB := b
	chA := a
	cf.Sessions[a] = &model.Session{Parent: &chB, Data: map[string]string{"k": "a"}}
	cf.Sessions[b] = &model.Session{Parent: &chA, Data: map[string]string{"k": "b"}}

	_, err := Get(cf, a, "k")
	if err != nil {
		t.Errorf("Get() error = %v, want nil (depth cap should prevent infinite loop)", err)
	}

	result, err := Resolve(cf, a)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if len(result) == 0 {
		t.Error("Resolve() expected keys from circular chain, got empty map")
	}
}

func TestResolve(t *testing.T) {
	cf := &model.ContextFile{
		Sessions: make(map[string]*model.Session),
	}

	grandparentID := "grandparent"
	cf.Sessions[grandparentID] = &model.Session{
		Parent: nil,
		Data:   map[string]string{"gp": "grandparent"},
	}

	parentID := "parent"
	chGrandparent := grandparentID
	cf.Sessions[parentID] = &model.Session{
		Parent: &chGrandparent,
		Data:   map[string]string{"p": "parent", "gp": "parent-overrides"},
	}

	childID := "child"
	chParent := parentID
	cf.Sessions[childID] = &model.Session{
		Parent: &chParent,
		Data:   map[string]string{"c": "child"},
	}

	result, err := Resolve(cf, childID)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result["c"] != "child" {
		t.Errorf("Resolve() c = %q, want %q", result["c"], "child")
	}

	if result["gp"] != "parent-overrides" {
		t.Errorf("Resolve() gp = %q, want %q (closer scope wins)", result["gp"], "parent-overrides")
	}

	if _, ok := result["p"]; !ok {
		t.Error("Resolve() missing key 'p' from parent")
	}
}
