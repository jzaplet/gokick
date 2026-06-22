-- +goose NO TRANSACTION
-- +goose Up
-- Add the users.tenant_id FK (-> tenants.id) and drop its DEFAULT. SQLite cannot
-- ALTER a column to add a foreign key or drop a default, so this is the standard
-- 12-step table rebuild. foreign_keys is turned OFF for the rebuild so DROP TABLE
-- users (referenced by refresh_tokens ON DELETE CASCADE) does NOT cascade-delete
-- the tokens. NO TRANSACTION + the MigrationManager's single-connection pin keep
-- the PRAGMA in effect across every statement below.
PRAGMA foreign_keys=OFF;

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

-- +goose Down
-- Reverse: rebuild users with the DEFAULT back and no FK.
PRAGMA foreign_keys=OFF;

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
    tenant_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);

INSERT INTO users_new (id, nickname, password_hash, email, role, active, created_at, updated_at, failed_login_attempts, last_failed_login_at, locked_until, tenant_id)
    SELECT id, nickname, password_hash, email, role, active, created_at, updated_at, failed_login_attempts, last_failed_login_at, locked_until, tenant_id
    FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

PRAGMA foreign_keys=ON;
