-- +goose Up
CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Bootstrap "Default" tenant. In single-tenant mode every user belongs to it
-- (shared.DefaultTenantID = nil UUID). It is created here, not by the seeder,
-- so it exists on EVERY startup before any user references it — required once
-- Krok 3 adds the users.tenant_id FK.
INSERT INTO tenants (id, name)
    VALUES ('00000000-0000-0000-0000-000000000000', 'Default')
    ON CONFLICT(id) DO NOTHING;

-- Every user carries a tenant. NOT NULL DEFAULT backfills existing rows and
-- stamps new single-tenant inserts automatically (the user repo INSERT omits
-- the column, so no creation path can write an empty value). Krok 3 adds the
-- composite indexes + FK and KEEPS this default. The default is dropped only in
-- Krok 4, together with Save writing tenant_id explicitly and every creation
-- path stamping the resolved tenant — dropping it sooner would make the column
-- NOT NULL with no default while Save still omits it, breaking every INSERT.
ALTER TABLE users
    ADD COLUMN tenant_id TEXT NOT NULL
    DEFAULT '00000000-0000-0000-0000-000000000000';

-- +goose Down
ALTER TABLE users DROP COLUMN tenant_id;
DROP TABLE IF EXISTS tenants;
