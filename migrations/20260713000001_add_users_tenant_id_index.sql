-- +goose Up
-- The users table had no index on tenant_id, so every tenant-scoped admin list
-- (WHERE tenant_id=? AND role!='superadmin' ORDER BY nickname) and the platform
-- per-tenant user count (GROUP BY tenant_id) fell back to a full table SCAN. The
-- composite (tenant_id, nickname) serves BOTH: the tenant_id prefix satisfies the
-- equality filter / GROUP BY, and the nickname suffix removes the ORDER BY sort
-- (no TEMP B-TREE). A plain CREATE INDEX needs no NO-TRANSACTION pragma dance.
CREATE INDEX idx_users_tenant_id_nickname ON users(tenant_id, nickname);

-- +goose Down
DROP INDEX idx_users_tenant_id_nickname;
