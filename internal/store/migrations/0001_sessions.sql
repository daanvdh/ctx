CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    parent_id TEXT,
    created_at DATETIME
);

CREATE TABLE IF NOT EXISTS session_data (
    session_id TEXT,
    key TEXT,
    value TEXT,
    PRIMARY KEY (session_id, key),
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
