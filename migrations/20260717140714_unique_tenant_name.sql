-- +goose Up
-- A tenant's name is its identity to an operator: the platform grid lists tenants
-- by name, and the Add-user form's tenant picker maps each option to {id, name} —
-- so two tenants sharing a name render as two identical, unpickable choices. There
-- was nothing stopping that: tenants.name carried no UNIQUE constraint and the
-- create path did no collision check, while `create-user --tenant-name X` CREATES
-- rather than resolves, so running it twice minted duplicates.
--
-- What makes it worth a constraint rather than a check alone is that the mistake is
-- not fixable in place: users.tenant_id is deliberately absent from the update
-- statement's SET clause (an edit must never move a user between tenants), so a
-- user filed into the wrong "Acme" can only be deleted and recreated — and in a
-- multitenant install they read the WRONG tenant's data until someone notices.
--
-- The index is the floor; CreateTenantHandler checks first so the operator gets a
-- 400 on the name field rather than a constraint violation surfacing as a 500.
--
-- NOTE for an existing deployment: if this migration fails on startup, the install
-- already HAS duplicate tenant names. That is the constraint doing its job — they
-- must be merged or renamed by hand first. There is no safe automatic answer,
-- because which of two same-named tenants a user belongs to is a question only an
-- operator can settle.
CREATE UNIQUE INDEX idx_tenants_name ON tenants(name);

-- +goose Down
DROP INDEX IF EXISTS idx_tenants_name;
