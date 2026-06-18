package store

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ctx/internal/model"
)

func TestLoadEmptyDB(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_load_empty.db")
	cf, err := Load(dsn)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cf.Sessions == nil {
		t.Fatal("Sessions map is nil")
	}
	if len(cf.Sessions) != 0 {
		t.Fatalf("expected empty sessions, got %d", len(cf.Sessions))
	}
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_save_load.db")
	parent := "parent-id"
	cfWrite := &model.ContextFile{Sessions: map[string]*model.Session{
		"session1": {Parent: &parent, Created: time.Now(), Data: map[string]string{"key": "value"}},
	}}

	if err := Save(dsn, cfWrite); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	cfRead, err := Load(dsn)
	if err != nil {
		t.Fatalf("Load after save error: %v", err)
	}
	if len(cfRead.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(cfRead.Sessions))
	}
	s, ok := cfRead.Sessions["session1"]
	if !ok {
		t.Fatal("session1 missing")
	}
	if s.Parent == nil || *s.Parent != parent {
		t.Fatalf("parent mismatch: got %v want %s", s.Parent, parent)
	}
	if v := s.Data["key"]; v != "value" {
		t.Fatalf("data mismatched: got %s want value", v)
	}
}

func TestConcurrentWrites(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_concurrent.db")
	n := 10
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("session_%d", id)
			if err := CreateSession(dsn, sessionID, nil); err != nil {
				errCh <- fmt.Errorf("create: %w", err)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("error: %v", err)
	}

	cf, err := Load(dsn)
	if err != nil {
		t.Fatalf("load after concurrent writes error: %v", err)
	}
	if len(cf.Sessions) != n {
		t.Fatalf("expected %d sessions, got %d", n, len(cf.Sessions))
	}
}

func TestSetValueCreatesAndUpdatesKey(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_set_value.db")

	if err := CreateSession(dsn, "s1", nil); err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if err := SetValue(dsn, "s1", "KEY", "first"); err != nil {
		t.Fatalf("SetValue insert error: %v", err)
	}
	if err := SetValue(dsn, "s1", "KEY", "second"); err != nil {
		t.Fatalf("SetValue update error: %v", err)
	}

	cf, err := Load(dsn)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got := cf.Sessions["s1"].Data["KEY"]; got != "second" {
		t.Fatalf("KEY = %q, want second", got)
	}
}

func TestDeleteSessionTree(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_delete_tree.db")

	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	parent := "root"
	if err := CreateSession(dsn, "child", &parent); err != nil {
		t.Fatalf("CreateSession child error: %v", err)
	}
	if err := CreateSession(dsn, "sibling", nil); err != nil {
		t.Fatalf("CreateSession sibling error: %v", err)
	}
	if err := SetValue(dsn, "child", "KEY", "value"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}

	if err := DeleteSessionTree(dsn, "root"); err != nil {
		t.Fatalf("DeleteSessionTree error: %v", err)
	}

	cf, err := Load(dsn)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if _, ok := cf.Sessions["root"]; ok {
		t.Fatal("root still exists after delete")
	}
	if _, ok := cf.Sessions["child"]; ok {
		t.Fatal("child still exists after delete")
	}
	if _, ok := cf.Sessions["sibling"]; !ok {
		t.Fatal("sibling should not be deleted")
	}
}
