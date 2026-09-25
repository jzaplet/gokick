-- +goose Up
-- The Postgres twin of migrations/sqlite/20260327000001_init_schema.sql — the
-- same logical schema, in native types, plus the row-level security that makes
-- Postgres enforce tenant isolation on its own. Every migration exists once per
-- dialect under the SAME version (a gate in app/zz_migrations_test.go fails on a
-- missing twin), so a deployment reaches the same schema whichever adapter it runs.
--
-- It runs as gokick_owner (APP_DB_MIGRATE_URL), which owns every object below.
-- The application never connects as the owner: it uses gokick_app (the tenant
-- plane, subject to the policies) and gokick_system (BYPASSRLS, the cross-tenant
-- plane). The roles themselves are cluster objects created once by
-- docker/postgres/initdb/01-roles.sh; this file only grants to them.
--
-- Types versus SQLite: ids are uuid (UUIDv7, minted by the application), times
-- are timestamptz, flags boolean, run payloads bytea and audit metadata jsonb.
-- Column DEFAULTs use statement_timestamp(), never now(): now() is the
-- transaction's start, which inside a long transaction would stamp rows with a
-- time far in the past.

-- Czech sort order for grids (ORDER BY … COLLATE app_sort), the ICU twin of the
-- SQLite adapter's app_sort collation — both follow the CLDR cs rules, pinned by
-- the golden corpus in app/infrastructure/database/testdata/sort_cs. Used in
-- queries only, never in a column definition or an index: equality and UNIQUE stay
-- binary. Deterministic, so it compares equal only what is byte-equal.
CREATE COLLATION app_sort (provider = icu, locale = 'cs-CZ');

-- Tenant registry (row-level multitenancy boundary; see /gk-multitenancy).
CREATE TABLE tenants (
    id         uuid PRIMARY KEY,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    plan       text NOT NULL DEFAULT 'free'
);

-- A tenant's name is its identity to an operator (see the SQLite twin); binary,
-- case-sensitive uniqueness.
CREATE UNIQUE INDEX idx_tenants_name ON tenants (name);

-- Bootstrap "Default" tenant (shared.DefaultTenantID). In single-tenant mode every
-- row belongs to it, and the tenant plane scopes to it.
INSERT INTO tenants (id, name)
    VALUES ('00000000-0000-0000-0000-000000000000', 'Default')
    ON CONFLICT (id) DO NOTHING;

-- Accounts. nickname is unique across ALL tenants, exactly as on SQLite — the
-- constraint holds under RLS too, since a unique check sees every row.
CREATE TABLE users (
    id                    uuid PRIMARY KEY,
    nickname              text NOT NULL UNIQUE,
    password_hash         text NOT NULL,
    email                 text,
    role                  text NOT NULL DEFAULT 'user'
                              CHECK (role IN ('superadmin', 'admin', 'user')),
    active                boolean NOT NULL DEFAULT true,
    created_at            timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at            timestamptz NOT NULL DEFAULT statement_timestamp(),
    failed_login_attempts integer NOT NULL DEFAULT 0,
    last_failed_login_at  timestamptz,
    locked_until          timestamptz,
    tenant_id             uuid NOT NULL REFERENCES tenants (id),
    last_login_at         timestamptz,
    lang                  text
);

-- Tenant-scoped lists: the tenant_id prefix scopes, the nickname suffix orders.
-- It also serves the users.tenant_id foreign key.
CREATE INDEX idx_users_tenant_id_nickname ON users (tenant_id, nickname);

-- Opaque refresh tokens: only the SHA-256 hash is stored. token_hash's UNIQUE
-- constraint is its index (the SQLite twin carries a redundant second one).
CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp()
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

-- Append-only security/audit trail, written outside the business transaction.
-- actor_user_id, target_id and actor_ip stay text: they hold whatever identifies
-- the actor or target, and an audit write must never fail on the shape of a value.
CREATE TABLE audit_log (
    id            uuid PRIMARY KEY,
    actor_user_id text,
    actor_ip      text,
    action        text NOT NULL,
    target_type   text,
    target_id     text,
    metadata      jsonb,
    created_at    timestamptz NOT NULL DEFAULT statement_timestamp()
);

CREATE INDEX idx_audit_log_action ON audit_log (action);
CREATE INDEX idx_audit_log_actor ON audit_log (actor_user_id);
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at);

-- The durable-task primitive — see the SQLite twin for the state machine and the
-- three counters. Differences: tenant_id is a real foreign key (a tenant with
-- pending runs cannot be deleted from under them), and the claim index is a plain
-- partial index on run_at, as timestamptz compares natively.
CREATE TABLE runs (
    id               uuid PRIMARY KEY,
    kind             text NOT NULL,
    tenant_id        uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
                         REFERENCES tenants (id),
    payload          bytea NOT NULL,
    state            bytea,
    run_at           timestamptz NOT NULL,
    attempts         integer NOT NULL DEFAULT 0,
    reclaims         integer NOT NULL DEFAULT 0,
    parks            integer NOT NULL DEFAULT 0,
    max_retries      integer NOT NULL DEFAULT 0,
    locked_by        text,
    locked_until     timestamptz,
    last_error       text,
    failed_at        timestamptz,
    completed_at     timestamptz,
    cancel_requested boolean NOT NULL DEFAULT false,
    cancelled_at     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at       timestamptz NOT NULL DEFAULT statement_timestamp(),
    lang             text NOT NULL DEFAULT 'en'
);

CREATE INDEX idx_runs_claim ON runs (run_at)
    WHERE completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL;
CREATE INDEX idx_runs_kind ON runs (kind, created_at);
-- Serves the tenant foreign key: deleting a tenant checks it owns no runs.
CREATE INDEX idx_runs_tenant_id ON runs (tenant_id);

-- Row-level security: the database's own tenant wall.
--
-- The tenant plane (gokick_app) opens every transaction with
-- set_config('app.tenant_id', <tenant>, true) — transaction-local, so the value
-- never leaks to the next user of a pooled connection. A query that forgets its
-- WHERE tenant_id = … then sees only its own tenant, and a write into another
-- tenant fails the WITH CHECK. Without the setting it sees nothing at all.
--
-- NULLIF is required: once a connection has run a transaction that set the value,
-- current_setting returns '' (not NULL) afterwards, and ''::uuid would raise an
-- error instead of matching no row.
CREATE FUNCTION gokick_current_tenant() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE
    AS $$ SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid $$;

-- Row security is ENABLEd, not FORCEd: the policies bind every role except the
-- owner and BYPASSRLS roles. Leaving the owner out is deliberate — a later data
-- migration (UPDATE users SET … as gokick_owner) must see every row, and under
-- FORCE it would silently match none. The application never runs as the owner:
-- at startup the adapter refuses an APP_DB_URL role that owns, or is a member of
-- the owner of, any table.
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE runs ENABLE ROW LEVEL SECURITY;
-- No policy on audit_log at all: row security with no policy denies every row to
-- a role that is subject to it, a second lock behind the missing grant.
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;

-- A tenant sees its own registry row, read-only (creating, renaming and deleting
-- tenants is platform work, on the system plane).
CREATE POLICY tenant_isolation ON tenants FOR SELECT
    USING (id = gokick_current_tenant());

CREATE POLICY tenant_isolation ON users
    USING (tenant_id = gokick_current_tenant())
    WITH CHECK (tenant_id = gokick_current_tenant());

CREATE POLICY tenant_isolation ON runs
    USING (tenant_id = gokick_current_tenant())
    WITH CHECK (tenant_id = gokick_current_tenant());

-- A token belongs to the tenant of its user: it is visible — and writable —
-- exactly when its user is. The subquery runs under the users policy, so a token
-- of another tenant's user matches nothing. (Logout revokes the caller's own
-- tokens inside its tenant transaction; pre-authentication work — login, refresh,
-- the expiry sweep — runs on the system plane.)
CREATE POLICY tenant_isolation ON refresh_tokens
    USING (EXISTS (SELECT 1 FROM users u WHERE u.id = refresh_tokens.user_id))
    WITH CHECK (EXISTS (SELECT 1 FROM users u WHERE u.id = refresh_tokens.user_id));

-- Grants. The tenant plane gets no access to audit_log at all, and the system
-- plane may only append to it and read it back: the log is append-only because the
-- database says so, not by convention.
GRANT SELECT ON tenants TO gokick_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON users, refresh_tokens, runs TO gokick_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, users, refresh_tokens, runs TO gokick_system;
GRANT SELECT, INSERT ON audit_log TO gokick_system;

-- +goose Down
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
DROP FUNCTION IF EXISTS gokick_current_tenant();
DROP COLLATION IF EXISTS app_sort;
