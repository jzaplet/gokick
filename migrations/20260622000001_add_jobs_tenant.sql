-- +goose Up
-- Jobs carry the tenant they were enqueued for. The worker bypasses the bus
-- (TenantMiddleware never runs for job handlers), so it restores this tenant
-- into the handler's context from the claimed row. NOT NULL DEFAULT backfills
-- existing rows and is a safe fallback; the dispatcher stamps the real tenant
-- from the enqueuing context. ClaimDue stays a global drain (no tenant filter),
-- so this column is never used to scope the claim — only to propagate.
ALTER TABLE jobs
    ADD COLUMN tenant_id TEXT NOT NULL
    DEFAULT '00000000-0000-0000-0000-000000000000';

-- +goose Down
ALTER TABLE jobs DROP COLUMN tenant_id;
