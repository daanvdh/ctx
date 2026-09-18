CREATE TABLE IF NOT EXISTS trigger_schedule_state (
    trigger_path TEXT PRIMARY KEY,
    last_fired_at DATETIME NOT NULL
);
