package store

import (
	"context"
	"database/sql"
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

func TestMigrationRecordsSchemaVersion(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_migration.db")

	if _, err := Load(dsn); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 5 {
		t.Fatalf("schema version = %d, want 5", version)
	}
}

func TestClaimTriggerScheduleFirstClaimSucceeds(t *testing.T) {
	tmp := t.TempDir()
	s := NewSQLite(filepath.Join(tmp, "test_claim.db"))
	dueAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	claimed, err := s.ClaimTriggerSchedule(context.Background(), "/triggers/poll.md", dueAt)
	if err != nil {
		t.Fatalf("ClaimTriggerSchedule error: %v", err)
	}
	if !claimed {
		t.Fatal("claimed = false, want true for first claim")
	}
}

func TestClaimTriggerScheduleSameOrEarlierDueAtFails(t *testing.T) {
	tmp := t.TempDir()
	s := NewSQLite(filepath.Join(tmp, "test_claim.db"))
	ctx := context.Background()
	dueAt := time.Date(2026, 7, 15, 12, 1, 0, 0, time.UTC)

	if claimed, err := s.ClaimTriggerSchedule(ctx, "/triggers/poll.md", dueAt); err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v, want true, nil", claimed, err)
	}

	if claimed, err := s.ClaimTriggerSchedule(ctx, "/triggers/poll.md", dueAt); err != nil {
		t.Fatalf("same dueAt claim error: %v", err)
	} else if claimed {
		t.Fatal("same dueAt claim succeeded, want false (already claimed)")
	}

	earlier := dueAt.Add(-time.Minute)
	if claimed, err := s.ClaimTriggerSchedule(ctx, "/triggers/poll.md", earlier); err != nil {
		t.Fatalf("earlier dueAt claim error: %v", err)
	} else if claimed {
		t.Fatal("earlier dueAt claim succeeded, want false")
	}
}

func TestClaimTriggerScheduleLaterDueAtSucceeds(t *testing.T) {
	tmp := t.TempDir()
	s := NewSQLite(filepath.Join(tmp, "test_claim.db"))
	ctx := context.Background()
	dueAt := time.Date(2026, 7, 15, 12, 1, 0, 0, time.UTC)

	if claimed, err := s.ClaimTriggerSchedule(ctx, "/triggers/poll.md", dueAt); err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v, want true, nil", claimed, err)
	}

	later := dueAt.Add(time.Minute)
	if claimed, err := s.ClaimTriggerSchedule(ctx, "/triggers/poll.md", later); err != nil {
		t.Fatalf("later dueAt claim error: %v", err)
	} else if !claimed {
		t.Fatal("later dueAt claim failed, want true")
	}
}

func TestClaimTriggerScheduleConcurrentClaimsOnlyOneWins(t *testing.T) {
	tmp := t.TempDir()
	s := NewSQLite(filepath.Join(tmp, "test_claim.db"))
	ctx := context.Background()
	dueAt := time.Date(2026, 7, 15, 12, 1, 0, 0, time.UTC)

	const attempts = 8
	results := make([]bool, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claimed, err := s.ClaimTriggerSchedule(ctx, "/triggers/poll.md", dueAt)
			if err != nil {
				t.Errorf("claim %d error: %v", i, err)
				return
			}
			results[i] = claimed
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, r := range results {
		if r {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly 1", wins)
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

func TestSetEntryStoresValueType(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_set_entry.db")

	if err := CreateSession(dsn, "s1", nil); err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if err := SetEntry(dsn, "s1", "REF", model.NewEntry("/tmp/some/path", model.ValueTypeFileRef)); err != nil {
		t.Fatalf("SetEntry error: %v", err)
	}

	entry, err := GetEntry(dsn, "s1", "REF")
	if err != nil {
		t.Fatalf("GetEntry error: %v", err)
	}
	if entry.ValueType != model.ValueTypeFileRef || entry.Value != "/tmp/some/path" {
		t.Fatalf("entry = %#v, want file_ref content", entry)
	}

	resolved, err := ResolveEntries(dsn, "s1")
	if err != nil {
		t.Fatalf("ResolveEntries error: %v", err)
	}
	if resolved["REF"].ValueType != model.ValueTypeFileRef {
		t.Fatalf("resolved REF type = %q, want file_ref", resolved["REF"].ValueType)
	}
}

// TestMigrateConvertsDocValueTypeToString simulates a pre-v4 database (schema
// version 3, with a legacy 'doc' value_type row) and verifies that opening it
// migrates that row to 'string'.
func TestMigrateConvertsDocValueTypeToString(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_migrate_doc.db")

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db error: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations error: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (3, ?)`, time.Now()); err != nil {
		t.Fatalf("seed schema version error: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (id TEXT PRIMARY KEY, parent_id TEXT, created_at DATETIME)`); err != nil {
		t.Fatalf("create sessions error: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE session_data (session_id TEXT, key TEXT, value TEXT, value_type TEXT NOT NULL DEFAULT 'string', PRIMARY KEY (session_id, key))`); err != nil {
		t.Fatalf("create session_data error: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE session_shares (from_session_id TEXT NOT NULL, to_session_id TEXT NOT NULL, created_at DATETIME NOT NULL, PRIMARY KEY (from_session_id, to_session_id))`); err != nil {
		t.Fatalf("create session_shares error: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, created_at) VALUES ('s1', ?)`, time.Now()); err != nil {
		t.Fatalf("seed session error: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO session_data (session_id, key, value, value_type) VALUES ('s1', 'KEY', 'hello', 'doc')`); err != nil {
		t.Fatalf("seed doc row error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db error: %v", err)
	}

	// Opening the db re-runs migrate(), which applies the v4 step against
	// the pre-existing schema version 3 database.
	entry, err := GetEntry(dsn, "s1", "KEY")
	if err != nil {
		t.Fatalf("GetEntry error: %v", err)
	}
	if entry.ValueType != model.ValueTypeString || entry.Value != "hello" {
		t.Fatalf("entry = %#v, want migrated string content", entry)
	}
}

func TestRemoveEntryDeletesSessionKey(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_remove_entry.db")

	if err := CreateSession(dsn, "s1", nil); err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if err := SetValue(dsn, "s1", "KEEP", "yes"); err != nil {
		t.Fatalf("SetValue KEEP error: %v", err)
	}
	if err := SetValue(dsn, "s1", "DROP", "no"); err != nil {
		t.Fatalf("SetValue DROP error: %v", err)
	}

	if err := RemoveEntry(dsn, "s1", "DROP"); err != nil {
		t.Fatalf("RemoveEntry error: %v", err)
	}

	if _, err := GetValue(dsn, "s1", "DROP"); err == nil {
		t.Fatal("expected removed key lookup to fail")
	}
	got, err := GetValue(dsn, "s1", "KEEP")
	if err != nil {
		t.Fatalf("GetValue KEEP error: %v", err)
	}
	if got != "yes" {
		t.Fatalf("KEEP = %q, want yes", got)
	}
}

func TestRemoveEntryMatchesWildcard(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_remove_entry_wildcard.db")

	if err := CreateSession(dsn, "s1", nil); err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if err := SetValue(dsn, "s1", "trigger_log.1", "a"); err != nil {
		t.Fatalf("SetValue trigger_log.1 error: %v", err)
	}
	if err := SetValue(dsn, "s1", "trigger_log.2", "b"); err != nil {
		t.Fatalf("SetValue trigger_log.2 error: %v", err)
	}
	if err := SetValue(dsn, "s1", "KEEP", "yes"); err != nil {
		t.Fatalf("SetValue KEEP error: %v", err)
	}

	if err := RemoveEntry(dsn, "s1", "*trigger_log*"); err != nil {
		t.Fatalf("RemoveEntry error: %v", err)
	}

	if _, err := GetValue(dsn, "s1", "trigger_log.1"); err == nil {
		t.Fatal("expected trigger_log.1 to be removed")
	}
	if _, err := GetValue(dsn, "s1", "trigger_log.2"); err == nil {
		t.Fatal("expected trigger_log.2 to be removed")
	}
	got, err := GetValue(dsn, "s1", "KEEP")
	if err != nil {
		t.Fatalf("GetValue KEEP error: %v", err)
	}
	if got != "yes" {
		t.Fatalf("KEEP = %q, want yes", got)
	}
}

func TestRemoveEntryDoesNotRemoveAncestorKey(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_remove_entry_ancestor.db")

	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	parent := "root"
	if err := CreateSession(dsn, "child", &parent); err != nil {
		t.Fatalf("CreateSession child error: %v", err)
	}
	if err := SetValue(dsn, "root", "KEY", "root-value"); err != nil {
		t.Fatalf("SetValue root error: %v", err)
	}
	if err := SetValue(dsn, "child", "KEY", "child-value"); err != nil {
		t.Fatalf("SetValue child error: %v", err)
	}

	if err := RemoveEntry(dsn, "child", "KEY"); err != nil {
		t.Fatalf("RemoveEntry error: %v", err)
	}

	got, err := GetValue(dsn, "child", "KEY")
	if err != nil {
		t.Fatalf("GetValue child error: %v", err)
	}
	if got != "root-value" {
		t.Fatalf("KEY = %q, want inherited root-value", got)
	}
}

func TestRemoveEntryErrorsWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_remove_entry_missing.db")

	if err := CreateSession(dsn, "s1", nil); err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if err := RemoveEntry(dsn, "s1", "MISSING"); err == nil {
		t.Fatal("expected missing entry error")
	}
	if err := RemoveEntry(dsn, "missing-session", "MISSING"); err == nil {
		t.Fatal("expected missing session error")
	}
}

func TestGetValueReadsNearestAncestor(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_get_value.db")

	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	parent := "root"
	if err := CreateSession(dsn, "child", &parent); err != nil {
		t.Fatalf("CreateSession child error: %v", err)
	}
	if err := SetValue(dsn, "root", "KEY", "root-value"); err != nil {
		t.Fatalf("SetValue root error: %v", err)
	}
	if err := SetValue(dsn, "child", "KEY", "child-value"); err != nil {
		t.Fatalf("SetValue child error: %v", err)
	}

	got, err := GetValue(dsn, "child", "KEY")
	if err != nil {
		t.Fatalf("GetValue error: %v", err)
	}
	if got != "child-value" {
		t.Fatalf("GetValue = %q, want child-value", got)
	}
}

func TestResolveReadsVisibleScope(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_resolve.db")

	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	parent := "root"
	if err := CreateSession(dsn, "child", &parent); err != nil {
		t.Fatalf("CreateSession child error: %v", err)
	}
	if err := SetValue(dsn, "root", "ROOT_ONLY", "root"); err != nil {
		t.Fatalf("SetValue root-only error: %v", err)
	}
	if err := SetValue(dsn, "root", "SHADOW", "root"); err != nil {
		t.Fatalf("SetValue root shadow error: %v", err)
	}
	if err := SetValue(dsn, "child", "SHADOW", "child"); err != nil {
		t.Fatalf("SetValue child shadow error: %v", err)
	}

	resolved, err := Resolve(dsn, "child")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if resolved["ROOT_ONLY"] != "root" {
		t.Fatalf("ROOT_ONLY = %q, want root", resolved["ROOT_ONLY"])
	}
	if resolved["SHADOW"] != "child" {
		t.Fatalf("SHADOW = %q, want child", resolved["SHADOW"])
	}
}

func TestShareContextReadsSharedBeforeAncestors(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_share_context.db")

	if err := CreateSession(dsn, "ancestor", nil); err != nil {
		t.Fatalf("CreateSession ancestor error: %v", err)
	}
	parent := "ancestor"
	if err := CreateSession(dsn, "target", &parent); err != nil {
		t.Fatalf("CreateSession target error: %v", err)
	}
	if err := CreateSession(dsn, "shared", nil); err != nil {
		t.Fatalf("CreateSession shared error: %v", err)
	}
	if err := SetValue(dsn, "ancestor", "KEY", "ancestor-value"); err != nil {
		t.Fatalf("SetValue ancestor error: %v", err)
	}
	if err := SetValue(dsn, "shared", "KEY", "shared-value"); err != nil {
		t.Fatalf("SetValue shared error: %v", err)
	}
	if err := ShareContext(dsn, "shared", "target"); err != nil {
		t.Fatalf("ShareContext error: %v", err)
	}

	got, err := GetValue(dsn, "target", "KEY")
	if err != nil {
		t.Fatalf("GetValue error: %v", err)
	}
	if got != "shared-value" {
		t.Fatalf("GetValue = %q, want shared-value", got)
	}
}

func TestShareContextTargetValueWins(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_share_target_wins.db")

	if err := CreateSession(dsn, "target", nil); err != nil {
		t.Fatalf("CreateSession target error: %v", err)
	}
	if err := CreateSession(dsn, "shared", nil); err != nil {
		t.Fatalf("CreateSession shared error: %v", err)
	}
	if err := SetValue(dsn, "target", "KEY", "target-value"); err != nil {
		t.Fatalf("SetValue target error: %v", err)
	}
	if err := SetValue(dsn, "shared", "KEY", "shared-value"); err != nil {
		t.Fatalf("SetValue shared error: %v", err)
	}
	if err := ShareContext(dsn, "shared", "target"); err != nil {
		t.Fatalf("ShareContext error: %v", err)
	}

	resolved, err := Resolve(dsn, "target")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if resolved["KEY"] != "target-value" {
		t.Fatalf("KEY = %q, want target-value", resolved["KEY"])
	}
}

func TestSessionNodes(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_session_nodes.db")

	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	parent := "root"
	if err := CreateSession(dsn, "child", &parent); err != nil {
		t.Fatalf("CreateSession child error: %v", err)
	}
	if err := SetValue(dsn, "child", "KEY", "value"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}

	nodes, err := SessionNodes(dsn)
	if err != nil {
		t.Fatalf("SessionNodes error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}
	if nodes[0].ID != "child" || nodes[0].Parent == nil || *nodes[0].Parent != "root" {
		t.Fatalf("first node = %+v, want child with root parent", nodes[0])
	}
	if nodes[0].Data["KEY"] != "value" {
		t.Fatalf("child KEY = %q, want value", nodes[0].Data["KEY"])
	}
}

func TestDeleteSessionRecursive(t *testing.T) {
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

	if err := DeleteSession(dsn, "root", false, false, false); err == nil {
		t.Fatal("expected non-recursive delete of session with children to fail")
	}

	if err := DeleteSession(dsn, "root", true, false, false); err != nil {
		t.Fatalf("DeleteSession recursive error: %v", err)
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

func TestDeleteSessionNonRecursive(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_delete_leaf.db")

	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	if err := SetValue(dsn, "root", "KEY", "value"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}

	if err := DeleteSession(dsn, "root", false, false, false); err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}

	cf, err := Load(dsn)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if _, ok := cf.Sessions["root"]; ok {
		t.Fatal("root still exists after delete")
	}
}

func TestDeleteSessionNonRecursiveNoVar(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_delete_novar.db")

	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	if err := SetValue(dsn, "root", "KEY", "value"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}

	if err := DeleteSession(dsn, "root", false, true, false); err == nil {
		t.Fatal("expected --no-var delete of session with variables to fail")
	}

	if err := DeleteSession(dsn, "root", false, false, false); err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}
}

// TestDeleteSessionRecursivePrune builds:
//
//	root
//	 |- a (has var)
//	 |   `- a1 (empty)
//	 `- b (empty)
//	     `- b1 (has var)
//
// With --recursive --no-var --no-child, only the fully empty leaf a1 qualifies
// for deletion bottom-up: b1 is kept for its variable, which keeps b (its
// parent) for still having a child, which in turn keeps root.
func TestDeleteSessionRecursivePrune(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test_delete_prune.db")

	root := "root"
	a, b := "a", "b"
	if err := CreateSession(dsn, "root", nil); err != nil {
		t.Fatalf("CreateSession root error: %v", err)
	}
	if err := CreateSession(dsn, "a", &root); err != nil {
		t.Fatalf("CreateSession a error: %v", err)
	}
	if err := CreateSession(dsn, "a1", &a); err != nil {
		t.Fatalf("CreateSession a1 error: %v", err)
	}
	if err := CreateSession(dsn, "b", &root); err != nil {
		t.Fatalf("CreateSession b error: %v", err)
	}
	if err := CreateSession(dsn, "b1", &b); err != nil {
		t.Fatalf("CreateSession b1 error: %v", err)
	}
	if err := SetValue(dsn, "a", "KEY", "value"); err != nil {
		t.Fatalf("SetValue a error: %v", err)
	}
	if err := SetValue(dsn, "b1", "KEY", "value"); err != nil {
		t.Fatalf("SetValue b1 error: %v", err)
	}

	if err := DeleteSession(dsn, "root", true, true, true); err != nil {
		t.Fatalf("DeleteSession prune error: %v", err)
	}

	cf, err := Load(dsn)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	for _, id := range []string{"root", "a", "b", "b1"} {
		if _, ok := cf.Sessions[id]; !ok {
			t.Fatalf("%s should not have been deleted", id)
		}
	}
	if _, ok := cf.Sessions["a1"]; ok {
		t.Fatal("a1 should have been deleted (empty leaf)")
	}
}
