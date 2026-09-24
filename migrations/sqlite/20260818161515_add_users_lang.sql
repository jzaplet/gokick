-- +goose Up
-- users.lang is the persisted UI-language preference (en canonical
-- + cs, extensible). It feeds one rung of the per-request resolution ladder
-- (X-App-Lang → cookie → users.lang → Accept-Language → en): the value is
-- minted into the JWT claims at session issue (login/refresh), and the claim
-- upgrades the request language when the client made no explicit choice.
-- Background work does NOT read this column — a run inherits the ENQUEUING
-- request's language via runs.lang; a future run that produces output for a
-- DIFFERENT user must load that user's preference explicitly. Login/refresh
-- also return it so the SPA can adopt the saved language right after sign-in
-- on a fresh browser.
--
-- NULLABLE on purpose: NULL means "no preference expressed" — the browser's
-- Accept-Language decides until the user picks a language (the profile
-- switcher stamps it; the future registration funnel will stamp the language
-- the visitor signed up in). No DEFAULT — an explicit choice is the only
-- writer. Validation to the supported set lives in the domain value object.
ALTER TABLE users ADD COLUMN lang TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN lang;
