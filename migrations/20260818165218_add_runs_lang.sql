-- +goose Up
-- runs.lang mirrors runs.tenant_id: work that executes OUTSIDE a request on a
-- user's behalf (a future mail send, a long agent turn) has no headers to
-- read, so the language rides on the row — stamped by the dispatcher from the
-- enqueuing request's context and restored into the handler context by the
-- worker, exactly the way the tenant already travels.
--
-- TEXT NOT NULL DEFAULT 'en': the dispatcher always stamps a resolved
-- language, so the default only covers pre-existing rows and direct repo
-- enqueues (debug endpoints, fixtures); the worker parses defensively and
-- falls back to the product default on anything unexpected.
ALTER TABLE runs ADD COLUMN lang TEXT NOT NULL DEFAULT 'en';

-- +goose Down
ALTER TABLE runs DROP COLUMN lang;
