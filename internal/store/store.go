package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	GetValue(ctx context.Context, sessionID, key string) (string, error)
	Resolve(ctx context.Context, sessionID string) (map[string]string, error)
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
	if current >= 1 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin migration: %w", err)
	}
	defer rollback(tx)

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
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?)`, time.Now()); err != nil {
		return fmt.Errorf("store: record schema migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration: %w", err)
	}
	return nil
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
		sess := &model.Session{Created: created, Data: make(map[string]string)}
		if parentID.Valid && parentID.String != "" {
			pid := parentID.String
			sess.Parent = &pid
		}
		cf.Sessions[id] = sess
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate sessions: %w", err)
	}

	dataRows, err := db.QueryContext(ctx, `SELECT session_id, key, value FROM session_data`)
	if err != nil {
		return nil, fmt.Errorf("store: query session_data: %w", err)
	}
	defer dataRows.Close()

	for dataRows.Next() {
		var sessionID, key, value string
		if err := dataRows.Scan(&sessionID, &key, &value); err != nil {
			return nil, fmt.Errorf("store: scan session_data: %w", err)
		}
		sess, ok := cf.Sessions[sessionID]
		if !ok {
			sess = &model.Session{Data: make(map[string]string)}
			cf.Sessions[sessionID] = sess
		}
		sess.Data[key] = value
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
			if _, err := tx.ExecContext(ctx, `INSERT INTO session_data (session_id, key, value) VALUES (?, ?, ?)`, id, key, value); err != nil {
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
	return retryBusy(ctx, func() error {
		return s.setValue(ctx, sessionID, key, value)
	})
}

func (s *SQLite) setValue(ctx context.Context, sessionID, key, value string) error {
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

	if _, err := tx.ExecContext(ctx, `
        INSERT INTO session_data (session_id, key, value)
        VALUES (?, ?, ?)
        ON CONFLICT(session_id, key) DO UPDATE SET value = excluded.value
    `, sessionID, key, value); err != nil {
		return fmt.Errorf("store: set data %s.%s: %w", sessionID, key, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func GetValue(path, sessionID, key string) (string, error) {
	return NewSQLite(path).GetValue(context.Background(), sessionID, key)
}

func (s *SQLite) GetValue(ctx context.Context, sessionID, key string) (string, error) {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return "", err
	}
	defer db.Close()

	if err := ensureSession(ctx, db, sessionID); err != nil {
		return "", err
	}

	const query = `
        WITH RECURSIVE chain(id, depth) AS (
            SELECT id, 0 FROM sessions WHERE id = ?
            UNION ALL
            SELECT sessions.parent_id, chain.depth + 1
            FROM sessions
            JOIN chain ON sessions.id = chain.id
            WHERE sessions.parent_id IS NOT NULL
              AND sessions.parent_id != ''
              AND chain.depth < 49
        )
        SELECT session_data.value
        FROM chain
        JOIN session_data ON session_data.session_id = chain.id
        WHERE session_data.key = ?
        ORDER BY chain.depth
        LIMIT 1`

	var value string
	if err := db.QueryRowContext(ctx, query, sessionID, key).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("key %s not found in session %s or ancestors", key, sessionID)
		}
		return "", fmt.Errorf("store: get data %s.%s: %w", sessionID, key, err)
	}
	return value, nil
}

func Resolve(path, sessionID string) (map[string]string, error) {
	return NewSQLite(path).Resolve(context.Background(), sessionID)
}

func (s *SQLite) Resolve(ctx context.Context, sessionID string) (map[string]string, error) {
	db, err := initDB(ctx, s.path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := ensureSession(ctx, db, sessionID); err != nil {
		return nil, err
	}

	const query = `
        WITH RECURSIVE chain(id, depth) AS (
            SELECT id, 0 FROM sessions WHERE id = ?
            UNION ALL
            SELECT sessions.parent_id, chain.depth + 1
            FROM sessions
            JOIN chain ON sessions.id = chain.id
            WHERE sessions.parent_id IS NOT NULL
              AND sessions.parent_id != ''
              AND chain.depth < 49
        )
        SELECT session_data.key, session_data.value
        FROM chain
        JOIN session_data ON session_data.session_id = chain.id
        ORDER BY chain.depth`

	rows, err := db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: resolve data for %s: %w", sessionID, err)
	}
	defer rows.Close()

	resolved := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("store: scan resolved data: %w", err)
		}
		if _, exists := resolved[key]; !exists {
			resolved[key] = value
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate resolved data: %w", err)
	}

	return resolved, nil
}

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
		node := &model.SessionNode{ID: id, Data: make(map[string]string)}
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

	dataRows, err := db.QueryContext(ctx, `SELECT session_id, key, value FROM session_data ORDER BY session_id, key`)
	if err != nil {
		return nil, fmt.Errorf("store: query session node data: %w", err)
	}
	defer dataRows.Close()

	for dataRows.Next() {
		var sessionID, key, value string
		if err := dataRows.Scan(&sessionID, &key, &value); err != nil {
			return nil, fmt.Errorf("store: scan session node data: %w", err)
		}
		if node, ok := nodesByID[sessionID]; ok {
			node.Data[key] = value
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
