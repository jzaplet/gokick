-- +goose Up
-- The ONE init migration. As a boilerplate, gokick ships its final schema as a
-- single migration — the incremental history (12 steps: jobs table + its drop,
-- users-table rebuilds for tenants/superadmin, …) was squashed 2026-07-15. New
-- deployments apply just this file; deployments that already ran the old chain
-- have this version recorded and skip it (their goose_db_version keeps the
-- historical entries — harmless). Project-specific migrations go in NEW files
-- after this one (make migrate-create).

-- Tenant registry (row-level multitenancy boundary; see /gk-multitenancy).
CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    plan TEXT NOT NULL DEFAULT 'free'
);

-- Bootstrap "Default" tenant. In single-tenant mode every user belongs to it
-- (shared.DefaultTenantID = nil UUID). It is created here, not by the seeder,
-- so it exists on EVERY startup before any user references it — required by
-- the users.tenant_id FK below.
INSERT INTO tenants (id, name)
    VALUES ('00000000-0000-0000-0000-000000000000', 'Default')
    ON CONFLICT(id) DO NOTHING;

-- Accounts. The role CHECK is the DB-level floor for the 3-tier ladder
-- (superadmin > admin > user); gating who may CREATE a superadmin is the
-- application/CLI layer's job. The brute-force lock columns are written on the
-- raw pool (outside any bus tx) — see /gk-repositories.
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    nickname TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('superadmin', 'admin', 'user')),
    active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    last_failed_login_at DATETIME,
    locked_until DATETIME,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    last_login_at DATETIME
);

-- Composite index: the tenant_id prefix serves tenant-scoped lists / GROUP BY,
-- the nickname suffix removes the ORDER BY sort (no TEMP B-TREE).
CREATE INDEX idx_users_tenant_id_nickname ON users(tenant_id, nickname);

-- Opaque refresh tokens: only the SHA-256 hash is stored (the raw value goes to
-- the client cookie); rotation + theft detection live in /gk-auth.
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

-- Append-only security/audit trail. Writes happen OUTSIDE the business
-- transaction (raw pool) so login failures and permission denials —
-- which roll back the business work — still leave a row here. Deliberately
-- NOT tenant-partitioned: a global control-plane log.
CREATE TABLE audit_log (
    id            TEXT PRIMARY KEY,
    actor_user_id TEXT,
    actor_ip      TEXT,
    action        TEXT NOT NULL,
    target_type   TEXT,
    target_id     TEXT,
    metadata      TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_actor ON audit_log(actor_user_id);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

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
CREATE TABLE runs (
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

-- Partial index over non-terminal runs; julianday() matches the repo-wide
-- datetime comparison idiom (see app/infrastructure/sqlite/sqltime.go).
CREATE INDEX idx_runs_claim ON runs(julianday(run_at), julianday(locked_until))
    WHERE completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL;
CREATE INDEX idx_runs_kind ON runs(kind, created_at);

-- +goose Down
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
