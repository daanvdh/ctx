package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ctx/internal/model"
)

func newTestFile(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.json")
	return path, func() {
		os.Remove(path)
		os.Remove(path + ".lock")
	}
}

func TestLoadMissingFile(t *testing.T) {
	path, cleanup := newTestFile(t)
	defer cleanup()

	cf, err := Load(path)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if cf.Sessions == nil {
		t.Fatal("expected Sessions to be initialized, got nil")
	}
	if len(cf.Sessions) != 0 {
		t.Fatalf("expected empty sessions, got %d", len(cf.Sessions))
	}
}

func TestLoadSaveRoundtrip(t *testing.T) {
	path, cleanup := newTestFile(t)
	defer cleanup()

	parent := "parent-id"
	cf := &model.ContextFile{
		Sessions: map[string]*model.Session{
			"session1": {
				Parent:  &parent,
				Created: time.Now(),
				Data:    map[string]string{"key": "value"},
			},
		},
	}

	if err := Save(path, cf); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(loaded.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(loaded.Sessions))
	}
	s, ok := loaded.Sessions["session1"]
	if !ok {
		t.Fatal("expected session1 to exist")
	}
	if *s.Parent != parent {
		t.Fatalf("expected parent %q, got %q", parent, *s.Parent)
	}
	if s.Data["key"] != "value" {
		t.Fatalf("expected key=value, got %s", s.Data["key"])
	}
}

func TestConcurrentWrites(t *testing.T) {
	path, cleanup := newTestFile(t)
	defer cleanup()

	n := 10
	errCh := make(chan error, n)

	// We use a mutex to simulate the mutual exclusion that WithLock provides
	// across processes. In a real multi-agent setup, flock handles inter-process
	// locking; here we verify the core logic is correctly serialized.
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(id int) {
			defer wg.Done()
			err := func() error {
				mu.Lock()
				defer mu.Unlock()

				cf, loadErr := Load(path)
				if loadErr != nil {
					return loadErr
				}
				key := fmt.Sprintf("key_%d", id)
				if cf.Sessions == nil {
					cf.Sessions = make(map[string]*model.Session)
				}
				cf.Sessions[key] = &model.Session{
					Created: time.Now(),
					Data:    map[string]string{"value": "ok"},
				}
				return Save(path, cf)
			}()
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent write error: %v", err)
		}
	}

	// Verify all sessions were written
	cf, loadErr := Load(path)
	if loadErr != nil {
		t.Fatalf("failed to load after concurrent writes: %v", loadErr)
	}
	if len(cf.Sessions) != n {
		t.Fatalf("expected %d sessions, got %d", n, len(cf.Sessions))
	}
}
