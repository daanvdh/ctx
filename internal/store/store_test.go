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
    var mu sync.Mutex
    errCh := make(chan error, n)

    for i := 0; i < n; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            mu.Lock()
            defer mu.Unlock()
            cf, err := Load(dsn)
            if err != nil {
                errCh <- fmt.Errorf("load: %w", err)
                return
            }
            key := fmt.Sprintf("key_%d", id)
            if cf.Sessions == nil {
                cf.Sessions = make(map[string]*model.Session)
            }
            cf.Sessions[key] = &model.Session{Created: time.Now(), Data: map[string]string{"value": "ok"}}
            if err := Save(dsn, cf); err != nil {
                errCh <- fmt.Errorf("save: %w", err)
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
