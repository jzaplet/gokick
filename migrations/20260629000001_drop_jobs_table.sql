-- +goose Up
-- Durable-task convergence: the `jobs` queue is absorbed by `runs` (the one
-- outside-transaction primitive). A "job" is now a run whose handler never
-- checkpoints; the job table/worker/dispatcher are removed. See the convergence
-- brief. No data migration: jobs are transient and the queue is empty in every
-- environment at cutover (no production enqueuers existed).
DROP INDEX IF EXISTS idx_jobs_kind;
DROP INDEX IF EXISTS idx_jobs_claim;
DROP TABLE IF EXISTS jobs;

-- +goose Down
-- Recreate the jobs table exactly as the two create migrations left it (base
-- columns + the tenant_id column), so a rollback restores a working schema.
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
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    tenant_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);

CREATE INDEX idx_jobs_claim ON jobs(run_at, locked_until)
    WHERE completed_at IS NULL AND failed_at IS NULL;

CREATE INDEX idx_jobs_kind ON jobs(kind, created_at);
