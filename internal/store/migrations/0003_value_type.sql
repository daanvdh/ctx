-- Guarded in Go: skipped when session_data.value_type already exists,
-- since sqlite has no ADD COLUMN IF NOT EXISTS and databases predating
-- schema versioning may already have the column.
ALTER TABLE session_data ADD COLUMN value_type TEXT NOT NULL DEFAULT 'string';
