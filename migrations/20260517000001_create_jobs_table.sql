-- +goose Up
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    payload BLOB NOT NULL,
    run_at DATETIME NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 0,
    locked_until DATETIME,
    last_error TEXT,
    failed_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Claim query scans pending rows ordered by run_at; locked_until disambiguates
-- in-progress vs free. Completed/failed rows are kept for audit but excluded
-- via completed_at/failed_at IS NULL guards in the application.
CREATE INDEX idx_jobs_claim ON jobs(run_at, locked_until)
    WHERE completed_at IS NULL AND failed_at IS NULL;

CREATE INDEX idx_jobs_kind ON jobs(kind, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_jobs_kind;
DROP INDEX IF EXISTS idx_jobs_claim;
DROP TABLE IF EXISTS jobs;
