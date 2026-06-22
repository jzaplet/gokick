-- +goose NO TRANSACTION
-- +goose Up
-- Widen the users.role CHECK to admit 'superadmin' (the platform plane added in
-- Krok 4c). SQLite cannot ALTER a CHECK constraint, so this is the same 12-step
-- table rebuild as the tenant_id FK migration — schema authored from a live
-- `.schema users`, changing ONLY the CHECK. The FK (-> tenants.id, no DEFAULT)
-- and every other column are preserved verbatim. foreign_keys is OFF so DROP
-- TABLE users does not cascade-delete refresh_tokens; NO TRANSACTION + the
-- MigrationManager single-connection pin keep the PRAGMA across all statements.
PRAGMA foreign_keys=OFF;

-- Guard against a leftover users_new from a previously-aborted run (NO
-- TRANSACTION means a mid-rebuild failure is not rolled back) so a retry is clean.
DROP TABLE IF EXISTS users_new;

CREATE TABLE users_new (
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
    tenant_id TEXT NOT NULL REFERENCES tenants(id)
);

INSERT INTO users_new (id, nickname, password_hash, email, role, active, created_at, updated_at, failed_login_attempts, last_failed_login_at, locked_until, tenant_id)
    SELECT id, nickname, password_hash, email, role, active, created_at, updated_at, failed_login_attempts, last_failed_login_at, locked_until, tenant_id
    FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

PRAGMA foreign_keys=ON;

-- +goose Down
-- Reverse: narrow the CHECK back to ('admin','user'), keeping the FK / no-default
-- (the post-Krok-4b state). NOTE: this Down FAILS BY DESIGN if any 'superadmin'
-- row still exists — the INSERT...SELECT copies it into a table whose CHECK
-- rejects it. Delete the superadmin account(s) before down-migrating. The
-- DROP IF EXISTS makes that retry clean after the expected first-attempt failure.
PRAGMA foreign_keys=OFF;

DROP TABLE IF EXISTS users_new;

CREATE TABLE users_new (
    id TEXT PRIMARY KEY,
    nickname TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    last_failed_login_at DATETIME,
    locked_until DATETIME,
    tenant_id TEXT NOT NULL REFERENCES tenants(id)
);

INSERT INTO users_new (id, nickname, password_hash, email, role, active, created_at, updated_at, failed_login_attempts, last_failed_login_at, locked_until, tenant_id)
    SELECT id, nickname, password_hash, email, role, active, created_at, updated_at, failed_login_attempts, last_failed_login_at, locked_until, tenant_id
    FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

PRAGMA foreign_keys=ON;
