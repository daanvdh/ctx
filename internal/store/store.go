package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"ctx/internal/model"
	_ "modernc.org/sqlite"
)

var mu sync.Mutex
var errAlreadyExists = errors.New("session already exists")

// initDB opens (or creates) the SQLite database at the given path and ensures
// that the required tables exist.
func initDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	// Set a busy timeout (ms) so SQLite will retry when the DB is locked.
	if _, err = db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return nil, fmt.Errorf("store: set busy_timeout: %w", err)
	}
	if _, err = db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return nil, fmt.Errorf("store: enable foreign_keys: %w", err)
	}

	// Create tables if they don't exist.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
        id TEXT PRIMARY KEY,
        parent_id TEXT,
        created_at DATETIME
    )`)
	if err != nil {
		return nil, fmt.Errorf("store: create sessions table: %w", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS session_data (
        session_id TEXT,
        key TEXT,
        value TEXT,
        PRIMARY KEY (session_id, key),
        FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
    )`)
	if err != nil {
		return nil, fmt.Errorf("store: create session_data table: %w", err)
	}
	// Return the opened DB.
	return db, nil

}

// Load reads all sessions from the SQLite database at path and returns a ContextFile.
func Load(path string) (*model.ContextFile, error) {
	mu.Lock()
	defer mu.Unlock()
	db, err := initDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	cf := &model.ContextFile{Sessions: make(map[string]*model.Session)}

	// Load sessions.
	rows, err := db.Query(`SELECT id, parent_id, created_at FROM sessions`)
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

	// Load data for each session.
	dRows, err := db.Query(`SELECT session_id, key, value FROM session_data`)
	if err != nil {
		return nil, fmt.Errorf("store: query session_data: %w", err)
	}
	defer dRows.Close()

	for dRows.Next() {
		var sid, k, v string
		if err := dRows.Scan(&sid, &k, &v); err != nil {
			return nil, fmt.Errorf("store: scan session_data: %w", err)
		}
		sess, ok := cf.Sessions[sid]
		if !ok {
			// Session without a row in sessions table – create placeholder.
			sess = &model.Session{Data: make(map[string]string)}
			cf.Sessions[sid] = sess
		}
		sess.Data[k] = v
	}
	if err := dRows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate session_data: %w", err)
	}

	return cf, nil
}

// Save writes the entire ContextFile to the SQLite database at path.
func Save(path string, cf *model.ContextFile) error {
	mu.Lock()
	defer mu.Unlock()
	db, err := initDB(path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	// Clear existing data.
	if _, err = tx.Exec(`DELETE FROM session_data`); err != nil {
		tx.Rollback()
		return fmt.Errorf("store: clear session_data: %w", err)
	}
	if _, err = tx.Exec(`DELETE FROM sessions`); err != nil {
		tx.Rollback()
		return fmt.Errorf("store: clear sessions: %w", err)
	}

	// Insert each session and its data.
	for id, s := range cf.Sessions {
		var parent sql.NullString
		if s.Parent != nil {
			parent = sql.NullString{String: *s.Parent, Valid: true}
		} else {
			parent = sql.NullString{Valid: false}
		}
		_, err := tx.Exec(`INSERT INTO sessions (id, parent_id, created_at) VALUES (?, ?, ?)`, id, parent, s.Created)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("store: insert session %s: %w", id, err)
		}
		for k, v := range s.Data {
			_, err := tx.Exec(`INSERT INTO session_data (session_id, key, value) VALUES (?, ?, ?)`, id, k, v)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("store: insert data %s.%s: %w", id, k, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func CreateSession(path, id string, parentID *string) error {
	mu.Lock()
	defer mu.Unlock()

	db, err := initDB(path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, id).Scan(&exists); err != nil {
		return fmt.Errorf("store: check session %s: %w", id, err)
	}
	if exists > 0 {
		return fmt.Errorf("%w: %s", errAlreadyExists, id)
	}

	var parent sql.NullString
	if parentID != nil {
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, *parentID).Scan(&exists); err != nil {
			return fmt.Errorf("store: check parent %s: %w", *parentID, err)
		}
		if exists == 0 {
			return fmt.Errorf("parent session %s not found", *parentID)
		}
		parent = sql.NullString{String: *parentID, Valid: true}
	}

	if _, err := tx.Exec(`INSERT INTO sessions (id, parent_id, created_at) VALUES (?, ?, ?)`, id, parent, time.Now()); err != nil {
		return fmt.Errorf("store: insert session %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func SetValue(path, sessionID, key, value string) error {
	mu.Lock()
	defer mu.Unlock()

	db, err := initDB(path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check session %s: %w", sessionID, err)
	}
	if exists == 0 {
		return fmt.Errorf("session %s not found", sessionID)
	}

	_, err = tx.Exec(`
        INSERT INTO session_data (session_id, key, value)
        VALUES (?, ?, ?)
        ON CONFLICT(session_id, key) DO UPDATE SET value = excluded.value
    `, sessionID, key, value)
	if err != nil {
		return fmt.Errorf("store: set data %s.%s: %w", sessionID, key, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func DeleteSessionTree(path, sessionID string) error {
	mu.Lock()
	defer mu.Unlock()

	db, err := initDB(path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer rollback(tx)

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
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

	if _, err := tx.Exec(descendants+` DELETE FROM session_data WHERE session_id IN (SELECT id FROM descendants)`, sessionID); err != nil {
		return fmt.Errorf("store: delete session data for %s: %w", sessionID, err)
	}
	if _, err := tx.Exec(descendants+` DELETE FROM sessions WHERE id IN (SELECT id FROM descendants)`, sessionID); err != nil {
		return fmt.Errorf("store: delete sessions for %s: %w", sessionID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

func IsAlreadyExists(err error) bool {
	return errors.Is(err, errAlreadyExists)
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}
