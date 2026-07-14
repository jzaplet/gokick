-- +goose Up
-- The one durable-task primitive: background work whose handler runs OUTSIDE
-- a transaction and persists resumable state via short checkpoint writes, with a
-- heartbeat renewing the lease — so a minutes/hours-long run never holds the
-- global SQLite write lock (which would freeze every other write).
--
-- State is DERIVED from columns (no status enum):
--   completed_at != NULL → done · failed_at != NULL → permanently failed
--   locked_until > now    → running (lease held) · otherwise → pending / retryable
--
-- locked_by is the owner token of the worker holding the lease. RenewLease,
-- Checkpoint and the terminal transitions are owner-checked (WHERE locked_by = ?)
-- so a worker that lost its lease (a stall past expiry) cannot stomp a run that
-- another worker has since claimed — fencing.
CREATE TABLE IF NOT EXISTS runs (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    -- Tenant the run was enqueued for; the worker restores it into the handler
    -- ctx from the claimed row (it bypasses the bus, so TenantMiddleware never
    -- ran). NOT NULL DEFAULT is the fallback; the dispatcher stamps the real
    -- tenant. ClaimDue is a global drain (no tenant filter) — this only propagates.
    tenant_id    TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    payload      BLOB NOT NULL,             -- immutable initial input
    state        BLOB,                      -- latest checkpoint (resumable state); NULL until first checkpoint
    run_at       DATETIME NOT NULL,         -- when eligible to claim (delay / retry backoff)
    -- Three distinct counters, deliberately NOT merged (a claim is NOT an attempt).
    -- A long run that merely straddles a deploy/OOM must not burn
    -- its logic-retry budget, so crash-reclaims and registry-skew parks are counted
    -- separately and each bounded independently of max_retries.
    attempts     INTEGER NOT NULL DEFAULT 0,  -- LOGIC retries: bumped on Reschedule; gated by max_retries
    reclaims     INTEGER NOT NULL DEFAULT 0,  -- CRASH reclaims: bumped when ClaimDue reclaims an EXPIRED lease; bounded by the worker, not by max_retries
    parks        INTEGER NOT NULL DEFAULT 0,  -- REGISTRY-SKEW parks: bumped when an unknown-kind run is parked during a rolling deploy; bounded by the worker, INDEPENDENT of max_retries (never consumes the logic-retry budget)
    max_retries  INTEGER NOT NULL DEFAULT 0,
    locked_by    TEXT,                      -- owner token of the worker holding the lease (a per-claim nonce)
    locked_until DATETIME,                  -- lease expiry; the heartbeat renews it
    last_error   TEXT,
    failed_at    DATETIME,
    completed_at DATETIME,
    -- Cancel is two-phase: an operator sets cancel_requested (the run keeps running
    -- so the worker can wind the handler down); the worker then records the terminal
    -- cancelled_at. cancel_requested rides on the row, so it survives a reclaim — a
    -- worker that picks up a crashed run honors a pending cancel.
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    cancelled_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP  -- bumped on every checkpoint / heartbeat
);

-- Claim scans pending rows ordered by run_at; locked_until disambiguates
-- in-progress vs free (an expired lease is reclaimable). Completed/failed rows
-- are kept for audit but excluded via the partial-index predicate.
--
-- The index keys the julianday() EXPRESSIONS, not the raw columns, because
-- ClaimDue compares and orders by julianday(run_at)/julianday(locked_until) (full
-- double precision — dodges strftime('%f') round-half-up skew). A raw-column index
-- would not match those expressions, forcing a full scan + a temp B-tree sort;
-- keying the expressions lets the claim do a bounded range seek on julianday(run_at)
-- and read it already ordered.
CREATE INDEX idx_runs_claim ON runs(julianday(run_at), julianday(locked_until))
    WHERE completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL;

CREATE INDEX idx_runs_kind ON runs(kind, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_runs_kind;
DROP INDEX IF EXISTS idx_runs_claim;
DROP TABLE IF EXISTS runs;
