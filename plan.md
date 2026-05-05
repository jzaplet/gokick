# Implementation plan

Isolated tests use real sqlite (`:memory:` or `t.TempDir()`), real bus, real handlers. No mocks.

## Phase 6: Auth

### Backend

- [x] **Task 1** — `application/auth/command/login.go` — `LoginCommand` (SkipPermission), `LoginResult`
- [x] **Task 2** — `LoginHandler.Handle()`: FindByNickname → Verify → GenerateAccessToken → GenerateRefreshToken → tokens.Save
  - Test: `login_handler_test.go` (real sqlite + seeded user) — success / wrong-pwd / unknown-nick / no-token-on-failure
- [x] **Task 3** — `application/auth/command/refresh_token.go` + handler
  - Rotation: old token `MarkUsed`, not deleted (for theft detection)
  - Theft detection: reuse of an already-used token → `DeleteByUserID` (force logout on all devices)
  - Test: success / expired / unknown / user-deleted / reuse-triggers-force-logout
- [x] **Task 4** — `application/auth/command/logout.go` + handler (Permissioned `auth:logout`): claims → `DeleteByUserID`
  - Test: deletes-all / does-not-touch-other-users / no-claims-returns-auth-error / required-permission
- [x] **Task 5** — `application/profile/command/change_password.go` + handler (Permissioned `profile:update`): claims → FindByID → Verify old → Hash new → Update
  - Test: success / wrong-old-password / no-claims / unknown-user / required-permission
  - **Refactor:** moved `testfx` from `auth/command/internal/` to `application/internal/` (shared across subpackages)
- [x] **Task 6** — `application/profile/query/get_profile.go` + handler (Permissioned `profile:read`): claims.UserID → `FindByID`
  - Test: success / no-claims / unknown-user / required-permission
- [x] **Task 7** — `domain/shared/permissions_registry.go` — `NewPermissionsRegistry([]Permissioned)` → `All()` / `ForRole(role) []string`
  - Shared helper `IsPermissionAllowedForRole(permission, role)` in `domain/shared/permission.go`, `PermissionChecker` now uses it (DRY)
  - Test: All sorted+dedup / ForRole(admin) = everything / ForRole(user) = without `admin:*` / unknown-role = like user / empty / copy-safety / 8 sub-tests for the helper
- [x] **Task 8** — `presentation/http/middleware/auth.go` — parse Bearer → `JwtService.Validate` → `ContextWithClaims`
  - **Error refactor:** `AuthError` 403→401, added `PermissionError` 403 (401 = not signed in, 403 = no permission)
  - **Fixture move:** `application/internal/testfx` → `app/internal/testfx` (shared across all `app/**` layers)
  - Test: valid-sets-claims / no-header-passes-through / missing-bearer-prefix-401 / invalid-token-401 / expired-401 / empty-bearer-401
- [x] **Task 9** — `presentation/http/handler/auth.go` — POST login / refresh / logout via bus
  - `config.CookieSecure` (env `APP_COOKIE_SECURE`, default true, dev false)
  - `testfx.NewBuses()` helper — production-configured stack of buses
  - Arch-lint: `deepScan: false` (false positives in DI wiring inside testfx)
  - 8 E2E tests (real bus + real SQLite): login success / invalid creds / malformed JSON / refresh valid cookie / refresh missing cookie / refresh invalid cookie clears / logout no-claims 401 / logout with claims 204
- [x] **Task 10** — ProfileHandler (Get, ChangePassword) + server registration of all routes
  - `presentation/http/handler/profile.go` — Get returns userDTO with permissions, ChangePassword 204
  - `server.go` — public routes + protected (JWT AuthMiddleware wrap) + SPA fallback
  - 7 E2E tests (get success user / get success admin / get no-claims 401 / change success / change wrong-old 401 / change malformed 400 / change no-claims 401)
- [x] **Task 11** — Wire DI: all auth/profile handlers, `CookieSecure` typed flag, `PermissionsRegistry` provider, `shared.JwtService` binding
  - `make dev` builds the binary, `./bin/app serve` starts up, curl smoke test login→profile→logout passed end-to-end
- [x] **Task 12** — Permissions in response (already done in Task 9/10: `registry.ForRole(role)` directly in the HTTP handler)

### Frontend

- [x] **Task 13** — 401 auto-refresh + retry
  - Refactor of Fetch/Auth to one-way layers (Fetch → Auth → Views), removed the auth bridge
  - `Fetch/`: 6 single-purpose files (apiFetch, apiUpload, apiDownload, accessToken, buildHeaders, parseResponse) + index
  - `Auth/`: 7 single-purpose files (state, login, logout, refresh, permissions, authFetch, useAuth) + index
  - `authFetch` = apiFetch + 401 retry + single-flight coalescing; skipped for `/api/v1/auth/*`
  - Tests: `tests/fetch/apiFetch.test.ts` (7) + `tests/auth/authFetch.test.ts` (5 integration, fetch is the only mock)
- [x] **Task 14** — `router.ts` — `authGuard` + required `meta.requiresAuth` (mirrors backend Permissioned/SkipPermission)
  - `AppRoute` type forces explicit `meta.requiresAuth: true|false` — TS rejects routes without a declaration
  - Runtime fail-closed for bypass (missing meta → redirect /home + error toast)
  - Redirect to `/login` with `?redirect=<path>`, toasts (info / error), admin shortcut via `hasPermission`
  - Stubs: `LoginView.vue`, `ProfileView.vue`, `AdminUsersView.vue` (Tasks 15/16/25 will extend)
  - Production routes: `/` + `/login` (public), `/profile` (auth), `/admin/users` (auth + permission)
  - 8 tests in `tests/router/authGuard.test.ts` (memory history + isolated state)
- [x] **Task 15** — `app/Auth/Views/LoginView.vue` — form nickname + password, calls `login()`
  - Error state: message from response into `Input` error slot + error toast
  - Redirect: `?redirect` query (from guard) or `/`
  - Success toast "Welcome back, {nickname}"
  - Loading state: disabled form + spinner in the button
- [x] **Task 16** — `app/Profile/Views/ProfileView.vue` — user info + change password form
  - Two cards: "Account information" (nickname, role) + "Change password" (old + new password)
  - PUT `/api/v1/profile/password` via `authFetch` with `{ old_password, new_password }`
  - Error handling follows the same pattern as LoginView (errors object, clearFieldError, general error box)
  - Success: toast "Password changed." + form reset

---

## Phase 7: Admin CRUD

- [x] **Task 17** — `application/user/command/create_user.go` + handler (Permissioned `admin:users:create`): Validate VO (nickname, role) → check not-empty (password, email) → check duplicate nickname → hash → save → collect `UserCreated`
  - 7 tests: success / duplicate-nickname (no event) / empty-nickname / invalid-role / empty-password / empty-email / required-permission
- [ ] **Task 18** — `application/user/command/update_user.go` + handler
  - Test: seed + update → changes persist
- [ ] **Task 19** — `application/user/command/delete_user.go` + handler — refuse self-delete
  - Test: seed + delete → 0 users / self-delete → ValidationError
- [ ] **Task 20** — `application/user/query/list_users.go` + handler
  - Test: seed 3 → expect 3 sorted
- [ ] **Task 21** — `application/user/event/send_welcome_email.go` — subscribe to `UserCreated` (just a log for now)
  - Test: dispatch event → log called
- [ ] **Task 22** — `presentation/http/middleware/role.go` — role guard (admin)
  - Test: httptest → claims.Role=admin → 200, user → 403
- [ ] **Task 23** — `presentation/http/handler/admin_users.go` — CRUD via bus
  - Test: httptest with real stack
- [ ] **Task 24** — Server registration of admin routes
- [ ] **Task 25** — `app/Admin/Views/AdminUsersView.vue` — list + modal create/edit/delete
- [ ] **Task 26** — `app/Home/Views/DashboardView.vue` — post-login page

---

## Other

- [ ] **Task 27** — `presentation/http/middleware/security.go` — security headers (HSTS, CSP, X-Frame-Options, Referrer-Policy, Permissions-Policy)
  - Test: httptest → all headers present
  - Goal: A+ rating on securityheaders.com
- [ ] **Task 28** — `Makefile: build-all` — cross-compile matrix (linux/amd64, darwin/arm64, windows/amd64)
- [ ] **Task 29** — `docker/release/Dockerfile` + `docker-compose.yml`
  - Smoke test: `docker compose up` → `/health` 200

---

## Phase: Queues and scheduled tasks

**Problem:** EventBus is in-memory and synchronous after commit. A crash between commit and dispatch = lost event. No retry, no delayed/recurring jobs, no visibility.

### Scheduled tasks (cron-like)

- [ ] **Task 30** — `infrastructure/scheduler/scheduler.go` — `Every(interval, name, fn)` + graceful shutdown
  - Started by the server, runs in a goroutine, respects `ctx.Done()`
  - Test: schedule fn → sleep → assert counter incremented
- [ ] **Task 31** — Register cleanup: `scheduler.Every(1*time.Hour, "cleanup:expired-tokens", tokens.DeleteExpired)`
  - Solves growth of `refresh_tokens` (after expiration they get deleted, both used tokens and the theft-detection window)

### Persistent job queue (SQLite)

- [ ] **Task 32** — Migration: `jobs` table (id, kind, payload JSON, run_at, attempts, max_attempts, locked_until, last_error, created_at, completed_at)
- [ ] **Task 33** — `domain/job/` — `Job` entity, `JobRepository` interface with `Enqueue`, `ClaimDue`, `Complete`, `Fail` methods
- [ ] **Task 34** — `infrastructure/sqlite/job/` — implementation with atomic claim via `UPDATE ... RETURNING`
- [ ] **Task 35** — `application/job/` — `JobDispatcher` (interface for enqueue from command handlers), `JobHandlerRegistry` (kind → fn mapping)
- [ ] **Task 36** — `infrastructure/worker/worker.go` — goroutine worker, poll interval, concurrency, exponential backoff, context cancellation
  - Test: enqueue fn → worker processes it → complete / enqueue failing fn → retry up to max_attempts
- [ ] **Task 37** — Integration with events: `DispatchEventsMiddleware` may optionally enqueue (for heavy handlers like email, external API)
  - Fast handlers (pure Go logic) stay synchronous
  - Heavy handlers have a `JobPayload` interface and are registered in `JobHandlerRegistry`

### CLI workflow

- [ ] **Task 38** — `./bin/app worker` — separate process for jobs only (for horizontal scaling; `./bin/app serve` may also turn the worker off via env)

---

## Progress

**Done:** 16 / 38 tasks — entire Phase 6 (Backend + Frontend) ✓

**Next:** Phase 7 — Admin CRUD (Task 17+)
