CREATE TABLE IF NOT EXISTS session_shares (
    from_session_id TEXT NOT NULL,
    to_session_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (from_session_id, to_session_id),
    FOREIGN KEY(from_session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY(to_session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
