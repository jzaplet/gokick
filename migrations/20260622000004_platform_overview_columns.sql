-- +goose Up
-- Columns powering the superadmin platform overview (Krok 4c). Kept OUT of the
-- risky role-CHECK rebuild (20260622000003) — these are plain ALTER ADD COLUMN,
-- safe inside a normal transaction.
--
-- users.last_login_at — stamped on each successful login (best-effort, raw pool).
-- tenants.plan        — free/paid tier; gokick ships the column + default, the
--                       product wires billing (Stripe) and the tenant_usage ledger.
ALTER TABLE users ADD COLUMN last_login_at DATETIME;
ALTER TABLE tenants ADD COLUMN plan TEXT NOT NULL DEFAULT 'free';

-- +goose Down
ALTER TABLE tenants DROP COLUMN plan;
ALTER TABLE users DROP COLUMN last_login_at;
