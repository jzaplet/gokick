# Roadmap

The boilerplate is functional end-to-end: DDD/CQRS backend, Vue 3 SPA, JWT auth with HttpOnly refresh cookie, admin user CRUD, role-based dashboards, security headers, persisted session across page refresh.

This file tracks what's left to make it production-ready and to grow it beyond the "starter" scope.


## Build & deploy

- [x] **Production Dockerfile** — `docker/production/Dockerfile` is a 3-stage multi-stage build: `node:24-alpine` Vite SPA build → `golang:1.26-alpine` Go binary build (embeds the SPA) → `alpine:3.20` minimal runtime that runs `serve`. Mount a host volume to `/data` in production for the SQLite file. `make docker-build` is a thin wrapper around `docker build` (no `make build` prerequisite).
- [x] **`docker-compose.yml`** — `app` service with named volume `app-data` and healthcheck on `/health`. `documan` stays as a separate service for local docs.
- [x] **GitHub CI** — `.github/workflows/validate.yml` is a single `validate` job: `make install` → `make lint` → `make test` → `make build`. Triggers on push to `main` and on pull requests. No image building in CI. Doc lint is skipped via `SKIP_DOCUMAN=1` (Documan needs Docker; locally it auto-starts via `make documan-lint`).
- [x] **Local Documan auto-start** — `make documan-{lint,fix,import,vectorize}` prefix each call with `docker compose up -d documan` (idempotent), so a fresh clone runs `make format` without manual setup.


## Background work

The current `EventBus` is in-memory and synchronous after transaction commit. A crash between commit and dispatch loses the event. There's no retry, no delayed/recurring jobs, and `refresh_tokens` grows forever (used + expired rows are never cleaned).

### In-process scheduler (cron-like)

- [ ] **`infrastructure/scheduler/scheduler.go`** — `Every(interval, name, fn)` with graceful shutdown via `ctx.Done()`. Started from the server, runs in its own goroutine.
- [ ] **Refresh token cleanup** — register `scheduler.Every(1*time.Hour, "cleanup:expired-tokens", tokens.DeleteExpired)`. Adds `DeleteExpired(ctx)` to `token.Repository` (criterion: `expires_at < now()` OR `used_at < now() - theftWindow`).

### Persistent job queue (SQLite)

For work that must survive crashes (welcome emails, external API calls, anything I/O-heavy or retry-prone).

- [ ] **Migration** — `jobs` table (`id`, `kind`, `payload` JSON, `run_at`, `attempts`, `max_attempts`, `locked_until`, `last_error`, `created_at`, `completed_at`).
- [ ] **`domain/job/`** — `Job` entity, `JobRepository` interface with `Enqueue`, `ClaimDue`, `Complete`, `Fail`.
- [ ] **`infrastructure/sqlite/job/`** — implementation, atomic claim via `UPDATE … RETURNING` to avoid double-processing.
- [ ] **`application/job/`** — `JobDispatcher` (interface that command handlers depend on for enqueueing), `JobHandlerRegistry` (kind → handler fn).
- [ ] **`infrastructure/worker/worker.go`** — goroutine pool, poll interval, concurrency, exponential backoff, respects context cancellation.
- [ ] **Event integration** — `DispatchEventsMiddleware` lets event handlers opt into queueing. Fast/pure handlers stay synchronous; heavy handlers register a `JobPayload` and run via the worker.

### CLI

- [ ] **`./bin/app worker`** — standalone process running only the job worker. Allows horizontal scaling (one `serve` process + N `worker` processes). `./bin/app serve` still runs an in-process worker by default; an env flag (`APP_INPROC_WORKER=false`) disables it for split deployments.


## Observability

- [ ] **Sentry** — `SENTRY_DSN` env, recovery middleware reports panics with trace_id + user_id context.
- [ ] **Structured slog attrs** — audit `request_id`, `user_id`, `command` consistency across all log lines emitted by middleware and handlers.
- [ ] **OpenTelemetry** — only when distributed tracing is needed. Otel HTTP middleware + propagation through bus middleware (trace_id is already a context-bound concept, OTel can replace the custom `TraceMiddleware`).


## Hardening

- [ ] **Rate limiting on auth endpoints** — token bucket or sliding window per IP for `POST /api/v1/auth/login` to slow brute-force.
- [ ] **Audit log** — append-only table for security-relevant actions (login success/failure, role changes, user deletes). Useful for compliance, low cost to add.
- [ ] **CSRF token endpoint** — currently `http.CrossOriginProtection` (Go 1.25 stdlib) handles same-site. If the SPA ever needs to be served from a different origin, the explicit double-submit cookie pattern would be needed.


## Notes

- Auth flow already done: login (cookie + access token), silent refresh on app boot, 401 auto-retry with single-flight refresh, theft detection via `used_at` marker.
- Admin user CRUD already done: list/create/update/delete with field-keyed validation errors, self-delete protection, role-change-on-self triggers full-page reload to refresh JWT.
- Migration is consolidated into a single `init_schema.sql` — fresh deploys are clean. Add new migrations as `20260328…` etc.
