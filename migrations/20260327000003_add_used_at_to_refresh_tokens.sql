-- +goose Up
ALTER TABLE refresh_tokens ADD COLUMN used_at DATETIME;

-- +goose Down
ALTER TABLE refresh_tokens DROP COLUMN used_at;
