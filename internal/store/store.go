package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"ctx/internal/model"
	_ "modernc.org/sqlite"
)

var ErrAlreadyExists = errors.New("session already exists")

type Store interface {
	Load(ctx context.Context) (*model.ContextFile, error)
	Save(ctx context.Context, cf *model.ContextFile) error
	CreateSession(ctx context.Context, id string, parentID *string) error
	SetValue(ctx context.Context, sessionID, key, value string) error
	SetEntry(ctx context.Context, sessionID, key string, entry model.Entry) error
	RemoveEntry(ctx context.Context, sessionID, key string) error
	GetValue(ctx context.Context, sessionID, key string) (string, error)
	GetEntry(ctx context.Context, sessionID, key string) (model.Entry, error)
	Resolve(ctx context.Context, sessionID string) (map[string]string, error)
	ResolveEntries(ctx context.Context, sessionID string) (map[string]model.Entry, error)
	ShareContext(ctx context.Context, fromSessionID, toSessionID string) error
	SessionNodes(ctx context.Context) ([]model.SessionNode, error)
	DeleteSessionTree(ctx context.Context, sessionID string) error
}

type SQLite struct {
	path string
}

func NewSQLite(path string) *SQLite {
	return &SQLite{path: path}
}

func initDB(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	if _, err = db.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: set busy_timeout: %w", err)
	}
	if _, err = db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: enable foreign_keys: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        applied_at DATETIME NOT NULL
    )`); err != nil {
		return fmt.Errorf("store: create schema_migrations table: %w", err)
	}

	var current int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if current >= 3 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin migration: %w", err)
	}
	defer rollback(tx)

	if current < 1 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sessions (
        id TEXT PRIMARY KEY,
        parent_id TEXT,
        created_at DATETIME
    )`); err != nil {
			return fmt.Errorf("store: create sessions table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS session_data (
        session_id TEXT,
        key TEXT,
        value TEXT,
        PRIMARY KEY (session_id, key),
        FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
    )`); err != nil {
			return fmt.Errorf("store: create session_data table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (1, ?)`, time.Now()); err != nil {
			return fmt.Errorf("store: record schema migration: %w", err)
		}
	}
	if current < 2 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS session_shares (
        from_session_id TEXT NOT NULL,
        to_session_id TEXT NOT NULL,
        created_at DATETIME NOT NULL,
        PRIMARY KEY (from_session_id, to_session_id),
        FOREIGN KEY(from_session_id) REFERENCES sessions(id) ON DELETE CASCADE,
        FOREIGN KEY(to_session_id) REFERENCES sessions(id) ON DELETE CASCADE
    )`); err != nil {
			return fmt.Errorf("store: create session_shares table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (2, ?)`, time.Now()); err != nil {
			return fmt.Errorf("store: record schema migration: %w", err)
		}
	}
	if current < 3 {
		hasColumn, err := columnExists(ctx, tx, "session_data", "value_type")
		if err != nil {
			return err
		}
		if !hasColumn {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE session_data ADD COLUMN value_type TEXT NOT NULL DEFAULT 'string'`); err != nil {
				return fmt.Errorf("store: add session_data.value_type: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (3, ?)`, time.Now()); err != nil {
			return fmt.Errorf("store: record schema migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration: %w", err)
	}
	return nil
}

func columnExists(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("store: inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dfltValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("store: scan %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func Load(path string) (*model.ContextFile, error) {
	return NewSQLite(path).Load(context.Background())
}

func (s *SQLite) Load(ctx context.Context) (*model.ContextFile, error) {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	cf := &model.ContextFile{Sessions: make(map[string]*model.Session)}

	rows, err := db.QueryContext(ctx, `SELECT id, parent_id, created_at FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("store: query sessions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var parentID sql.NullString
		var created time.Time
		if err := rows.Scan(&id, &parentID, &created); err != nil {
			return nil, fmt.Errorf("store: scan session: %w", err)
		}
		sess := &model.Session{Created: created, Data: make(map[string]string), Entries: make(map[string]model.Entry)}
		if parentID.Valid && parentID.String != "" {
			pid := parentID.String
			sess.Parent = &pid
		}
		cf.Sessions[id] = sess
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate sessions: %w", err)
	}

	dataRows, err := db.QueryContext(ctx, `SELECT session_id, key, value, value_type FROM session_data`)
	if err != nil {
		return nil, fmt.Errorf("store: query session_data: %w", err)
	}
	defer dataRows.Close()

	for dataRows.Next() {
		var sessionID, key, value string
		var valueType model.ValueType
		if err := dataRows.Scan(&sessionID, &key, &value, &valueType); err != nil {
			return nil, fmt.Errorf("store: scan session_data: %w", err)
		}
		sess, ok := cf.Sessions[sessionID]
		if !ok {
			sess = &model.Session{Data: make(map[string]string), Entries: make(map[string]model.Entry)}
			cf.Sessions[sessionID] = sess
		}
		sess.Data[key] = value
		sess.Entries[key] = model.NewEntry(value, valueType)
	}
	if err := dataRows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate session_data: %w", err)
	}

	return cf, nil
}

func Save(path string, cf *model.ContextFile) error {
	return NewSQLite(path).Save(context.Background(), cf)
}

func (s *SQLite) Save(ctx context.Context, cf *model.ContextFile) error {
	return retryBusy(ctx, func() error {
		return s.save(ctx, cf)
	})
}

func (s *SQLite) save(ctx context.Context, cf *model.ContextFile) error {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	if _, err = tx.ExecContext(ctx, `DELETE FROM session_data`); err != nil {
		return fmt.Errorf("store: clear session_data: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM session_shares`); err != nil {
		return fmt.Errorf("store: clear session_shares: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return fmt.Errorf("store: clear sessions: %w", err)
	}

	for id, sess := range cf.Sessions {
		var parent sql.NullString
		if sess.Parent != nil {
			parent = sql.NullString{String: *sess.Parent, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (id, parent_id, created_at) VALUES (?, ?, ?)`, id, parent, sess.Created); err != nil {
			return fmt.Errorf("store: insert session %s: %w", id, err)
		}
		for key, value := range sess.Data {
			entry := model.NewEntry(value, model.ValueTypeString)
			if sess.Entries != nil {
				if typed, ok := sess.Entries[key]; ok {
					entry = typed
				}
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO session_data (session_id, key, value, value_type) VALUES (?, ?, ?, ?)`, id, key, entry.Value, entry.ValueType); err != nil {
				return fmt.Errorf("store: insert data %s.%s: %w", id, key, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func CreateSession(path, id string, parentID *string) error {
	return NewSQLite(path).CreateSession(context.Background(), id, parentID)
}

func (s *SQLite) CreateSession(ctx context.Context, id string, parentID *string) error {
	return retryBusy(ctx, func() error {
		return s.createSession(ctx, id, parentID)
	})
}

func (s *SQLite) createSession(ctx context.Context, id string, parentID *string) error {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, id).Scan(&exists); err != nil {
		return fmt.Errorf("store: check session %s: %w", id, err)
	}
	if exists > 0 {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, id)
	}

	var parent sql.NullString
	if parentID != nil {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, *parentID).Scan(&exists); err != nil {
			return fmt.Errorf("store: check parent %s: %w", *parentID, err)
		}
		if exists == 0 {
			return fmt.Errorf("parent session %s not found", *parentID)
		}
		parent = sql.NullString{String: *parentID, Valid: true}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (id, parent_id, created_at) VALUES (?, ?, ?)`, id, parent, time.Now()); err != nil {
		return fmt.Errorf("store: insert session %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func SetValue(path, sessionID, key, value string) error {
	return NewSQLite(path).SetValue(context.Background(), sessionID, key, value)
}

func (s *SQLite) SetValue(ctx context.Context, sessionID, key, value string) error {
	return s.SetEntry(ctx, sessionID, key, model.NewEntry(value, model.ValueTypeString))
}

func SetEntry(path, sessionID, key string, entry model.Entry) error {
	return NewSQLite(path).SetEntry(context.Background(), sessionID, key, entry)
}

func (s *SQLite) SetEntry(ctx context.Context, sessionID, key string, entry model.Entry) error {
	return retryBusy(ctx, func() error {
		return s.setEntry(ctx, sessionID, key, entry)
	})
}

func (s *SQLite) setEntry(ctx context.Context, sessionID, key string, entry model.Entry) error {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check session %s: %w", sessionID, err)
	}
	if exists == 0 {
		return fmt.Errorf("session %s not found", sessionID)
	}

	entry = model.NewEntry(entry.Value, entry.ValueType)
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO session_data (session_id, key, value, value_type)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(session_id, key) DO UPDATE SET value = excluded.value, value_type = excluded.value_type
    `, sessionID, key, entry.Value, entry.ValueType); err != nil {
		return fmt.Errorf("store: set data %s.%s: %w", sessionID, key, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func RemoveEntry(path, sessionID, key string) error {
	return NewSQLite(path).RemoveEntry(context.Background(), sessionID, key)
}

func (s *SQLite) RemoveEntry(ctx context.Context, sessionID, key string) error {
	return retryBusy(ctx, func() error {
		return s.removeEntry(ctx, sessionID, key)
	})
}

func (s *SQLite) removeEntry(ctx context.Context, sessionID, key string) error {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check session %s: %w", sessionID, err)
	}
	if exists == 0 {
		return fmt.Errorf("session %s not found", sessionID)
	}

	keys := []string{key}
	if strings.ContainsAny(key, "*?[") {
		keys, err = matchingKeys(ctx, tx, sessionID, key)
		if err != nil {
			return err
		}
	}

	var removed int64
	for _, k := range keys {
		result, err := tx.ExecContext(ctx, `DELETE FROM session_data WHERE session_id = ? AND key = ?`, sessionID, k)
		if err != nil {
			return fmt.Errorf("store: remove data %s.%s: %w", sessionID, k, err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: inspect removed data %s.%s: %w", sessionID, k, err)
		}
		removed += n
	}
	if removed == 0 {
		return fmt.Errorf("entry %s not found in session %s", key, sessionID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func matchingKeys(ctx context.Context, tx *sql.Tx, sessionID, pattern string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT key FROM session_data WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: list keys for %s: %w", sessionID, err)
	}
	defer rows.Close()

	var matched []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("store: scan key for %s: %w", sessionID, err)
		}
		if ok, err := path.Match(pattern, k); err != nil {
			return nil, fmt.Errorf("store: invalid pattern %s: %w", pattern, err)
		} else if ok {
			matched = append(matched, k)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate keys for %s: %w", sessionID, err)
	}
	return matched, nil
}

func GetValue(path, sessionID, key string) (string, error) {
	return NewSQLite(path).GetValue(context.Background(), sessionID, key)
}

func (s *SQLite) GetValue(ctx context.Context, sessionID, key string) (string, error) {
	entry, err := s.GetEntry(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	return entry.Value, nil
}

func GetEntry(path, sessionID, key string) (model.Entry, error) {
	return NewSQLite(path).GetEntry(context.Background(), sessionID, key)
}

func (s *SQLite) GetEntry(ctx context.Context, sessionID, key string) (model.Entry, error) {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return model.Entry{}, err
	}
	defer db.Close()

	if err := ensureSession(ctx, db, sessionID); err != nil {
		return model.Entry{}, err
	}

	const query = visibleScopeCTE + `
        SELECT session_data.value, session_data.value_type
        FROM visible_scope
        JOIN session_data ON session_data.session_id = visible_scope.id
        WHERE session_data.key = ?
        ORDER BY visible_scope.priority, visible_scope.share_order, visible_scope.depth
        LIMIT 1`

	var value string
	var valueType model.ValueType
	if err := db.QueryRowContext(ctx, query, sessionID, sessionID, key).Scan(&value, &valueType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Entry{}, fmt.Errorf("key %s not found in session %s or ancestors", key, sessionID)
		}
		return model.Entry{}, fmt.Errorf("store: get data %s.%s: %w", sessionID, key, err)
	}
	return model.NewEntry(value, valueType), nil
}

func ShareContext(path, fromSessionID, toSessionID string) error {
	return NewSQLite(path).ShareContext(context.Background(), fromSessionID, toSessionID)
}

func (s *SQLite) ShareContext(ctx context.Context, fromSessionID, toSessionID string) error {
	return retryBusy(ctx, func() error {
		return s.shareContext(ctx, fromSessionID, toSessionID)
	})
}

func (s *SQLite) shareContext(ctx context.Context, fromSessionID, toSessionID string) error {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	for _, id := range []string{fromSessionID, toSessionID} {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, id).Scan(&exists); err != nil {
			return fmt.Errorf("store: check session %s: %w", id, err)
		}
		if exists == 0 {
			return fmt.Errorf("session %s not found", id)
		}
	}

	if _, err := tx.ExecContext(ctx, `
        INSERT INTO session_shares (from_session_id, to_session_id, created_at)
        VALUES (?, ?, ?)
        ON CONFLICT(from_session_id, to_session_id) DO NOTHING
    `, fromSessionID, toSessionID, time.Now()); err != nil {
		return fmt.Errorf("store: share context %s to %s: %w", fromSessionID, toSessionID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func Resolve(path, sessionID string) (map[string]string, error) {
	return NewSQLite(path).Resolve(context.Background(), sessionID)
}

func (s *SQLite) Resolve(ctx context.Context, sessionID string) (map[string]string, error) {
	entries, err := s.ResolveEntries(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]string, len(entries))
	for key, entry := range entries {
		resolved[key] = entry.Value
	}
	return resolved, nil
}

func ResolveEntries(path, sessionID string) (map[string]model.Entry, error) {
	return NewSQLite(path).ResolveEntries(context.Background(), sessionID)
}

func (s *SQLite) ResolveEntries(ctx context.Context, sessionID string) (map[string]model.Entry, error) {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := ensureSession(ctx, db, sessionID); err != nil {
		return nil, err
	}

	const query = visibleScopeCTE + `
        SELECT session_data.key, session_data.value, session_data.value_type
        FROM visible_scope
        JOIN session_data ON session_data.session_id = visible_scope.id
        ORDER BY visible_scope.priority, visible_scope.share_order, visible_scope.depth`

	rows, err := db.QueryContext(ctx, query, sessionID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: resolve data for %s: %w", sessionID, err)
	}
	defer rows.Close()

	resolved := make(map[string]model.Entry)
	for rows.Next() {
		var key, value string
		var valueType model.ValueType
		if err := rows.Scan(&key, &value, &valueType); err != nil {
			return nil, fmt.Errorf("store: scan resolved data: %w", err)
		}
		if _, exists := resolved[key]; !exists {
			resolved[key] = model.NewEntry(value, valueType)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate resolved data: %w", err)
	}

	return resolved, nil
}

const visibleScopeCTE = `
        WITH RECURSIVE
        own_chain(id, depth) AS (
            SELECT id, 0 FROM sessions WHERE id = ?
            UNION ALL
            SELECT sessions.parent_id, own_chain.depth + 1
            FROM sessions
            JOIN own_chain ON sessions.id = own_chain.id
            WHERE sessions.parent_id IS NOT NULL
              AND sessions.parent_id != ''
              AND own_chain.depth < 49
        ),
        shared_roots(id, share_order) AS (
            SELECT from_session_id, ROW_NUMBER() OVER (ORDER BY from_session_id)
            FROM session_shares
            WHERE to_session_id = ?
        ),
        shared_chain(id, share_order, depth) AS (
            SELECT id, share_order, 0 FROM shared_roots
            UNION ALL
            SELECT sessions.parent_id, shared_chain.share_order, shared_chain.depth + 1
            FROM sessions
            JOIN shared_chain ON sessions.id = shared_chain.id
            WHERE sessions.parent_id IS NOT NULL
              AND sessions.parent_id != ''
              AND shared_chain.depth < 49
        ),
        visible_scope(id, priority, share_order, depth) AS (
            SELECT id, 0, 0, 0 FROM own_chain WHERE depth = 0
            UNION ALL
            SELECT id, 1, share_order, depth FROM shared_chain
            UNION ALL
            SELECT id, 2, 0, depth FROM own_chain WHERE depth > 0
        )`

func SessionNodes(path string) ([]model.SessionNode, error) {
	return NewSQLite(path).SessionNodes(context.Background())
}

func (s *SQLite) SessionNodes(ctx context.Context) ([]model.SessionNode, error) {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT id, parent_id FROM sessions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: query session nodes: %w", err)
	}
	defer rows.Close()

	nodesByID := make(map[string]*model.SessionNode)
	order := []string{}
	for rows.Next() {
		var id string
		var parentID sql.NullString
		if err := rows.Scan(&id, &parentID); err != nil {
			return nil, fmt.Errorf("store: scan session node: %w", err)
		}
		node := &model.SessionNode{ID: id, Data: make(map[string]string), Entries: make(map[string]model.Entry)}
		if parentID.Valid && parentID.String != "" {
			parent := parentID.String
			node.Parent = &parent
		}
		nodesByID[id] = node
		order = append(order, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate session nodes: %w", err)
	}

	dataRows, err := db.QueryContext(ctx, `SELECT session_id, key, value, value_type FROM session_data ORDER BY session_id, key`)
	if err != nil {
		return nil, fmt.Errorf("store: query session node data: %w", err)
	}
	defer dataRows.Close()

	for dataRows.Next() {
		var sessionID, key, value string
		var valueType model.ValueType
		if err := dataRows.Scan(&sessionID, &key, &value, &valueType); err != nil {
			return nil, fmt.Errorf("store: scan session node data: %w", err)
		}
		if node, ok := nodesByID[sessionID]; ok {
			node.Data[key] = value
			node.Entries[key] = model.NewEntry(value, valueType)
		}
	}
	if err := dataRows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate session node data: %w", err)
	}

	nodes := make([]model.SessionNode, 0, len(order))
	for _, id := range order {
		nodes = append(nodes, *nodesByID[id])
	}
	return nodes, nil
}

func DeleteSessionTree(path, sessionID string) error {
	return NewSQLite(path).DeleteSessionTree(context.Background(), sessionID)
}

func (s *SQLite) DeleteSessionTree(ctx context.Context, sessionID string) error {
	return retryBusy(ctx, func() error {
		return s.deleteSessionTree(ctx, sessionID)
	})
}

func (s *SQLite) deleteSessionTree(ctx context.Context, sessionID string) error {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check session %s: %w", sessionID, err)
	}
	if exists == 0 {
		return fmt.Errorf("session %s not found", sessionID)
	}

	const descendants = `
        WITH RECURSIVE descendants(id) AS (
            SELECT id FROM sessions WHERE id = ?
            UNION ALL
            SELECT sessions.id
            FROM sessions
            JOIN descendants ON sessions.parent_id = descendants.id
        )`

	if _, err := tx.ExecContext(ctx, descendants+` DELETE FROM session_data WHERE session_id IN (SELECT id FROM descendants)`, sessionID); err != nil {
		return fmt.Errorf("store: delete session data for %s: %w", sessionID, err)
	}
	if _, err := tx.ExecContext(ctx, descendants+` DELETE FROM sessions WHERE id IN (SELECT id FROM descendants)`, sessionID); err != nil {
		return fmt.Errorf("store: delete sessions for %s: %w", sessionID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func ensureSession(ctx context.Context, db *sql.DB, sessionID string) error {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check session %s: %w", sessionID, err)
	}
	if exists == 0 {
		return fmt.Errorf("session %s not found", sessionID)
	}
	return nil
}

func retryBusy(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		err = fn()
		if !isBusy(err) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked")
}
