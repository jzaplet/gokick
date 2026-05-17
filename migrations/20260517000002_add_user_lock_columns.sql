-- +goose Up
-- Brute-force protection state for the login flow. failed_login_attempts
-- counts losing attempts inside the rolling window; locked_until is the
-- point after which the account is reachable again. Both default to a
-- non-locked / fresh state so existing users keep logging in unchanged.
ALTER TABLE users ADD COLUMN failed_login_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN last_failed_login_at DATETIME;
ALTER TABLE users ADD COLUMN locked_until DATETIME;

-- +goose Down
ALTER TABLE users DROP COLUMN locked_until;
ALTER TABLE users DROP COLUMN last_failed_login_at;
ALTER TABLE users DROP COLUMN failed_login_attempts;
