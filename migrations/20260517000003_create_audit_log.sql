-- +goose Up
-- Append-only security/audit trail. Writes happen OUTSIDE the business
-- transaction (raw pool) so login failures and permission denials —
-- which roll back the business work — still leave a row here.
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

-- +goose Down
DROP INDEX IF EXISTS idx_audit_log_created_at;
DROP INDEX IF EXISTS idx_audit_log_actor;
DROP INDEX IF EXISTS idx_audit_log_action;
DROP TABLE IF EXISTS audit_log;
