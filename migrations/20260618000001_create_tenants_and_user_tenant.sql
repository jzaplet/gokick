-- +goose Up
CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Bootstrap "Default" tenant. In single-tenant mode every user belongs to it
-- (shared.DefaultTenantID = nil UUID). It is created here, not by the seeder,
-- so it exists on EVERY startup before any user references it — required by the
-- users.tenant_id FK added in a later migration.
INSERT INTO tenants (id, name)
    VALUES ('00000000-0000-0000-0000-000000000000', 'Default')
    ON CONFLICT(id) DO NOTHING;

-- Every user carries a tenant. NOT NULL DEFAULT backfills existing rows and
-- stamps new inserts automatically while the user repo INSERT still omitted the
-- column. A later migration adds the FK and drops this default, once Save writes
-- tenant_id explicitly and every creation path stamps the resolved tenant —
-- dropping it sooner would make the column NOT NULL with no default while Save
-- still omitted it, breaking every INSERT.
ALTER TABLE users
    ADD COLUMN tenant_id TEXT NOT NULL
    DEFAULT '00000000-0000-0000-0000-000000000000';

-- +goose Down
ALTER TABLE users DROP COLUMN tenant_id;
DROP TABLE IF EXISTS tenants;
