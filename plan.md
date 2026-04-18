# Plán implementace

Izolované testy používají real sqlite (`:memory:` nebo `t.TempDir()`), real bus, real handlery. Žádné mocky.

## Fáze 6: Auth

### Backend

- [x] **Task 1** — `application/auth/command/login.go` — `LoginCommand` (SkipPermission), `LoginResult`
- [x] **Task 2** — `LoginHandler.Handle()`: FindByNickname → Verify → GenerateAccessToken → GenerateRefreshToken → tokens.Save
  - Test: `login_handler_test.go` (real sqlite + seeded user) — success / wrong-pwd / unknown-nick / no-token-on-failure
- [x] **Task 3** — `application/auth/command/refresh_token.go` + handler
  - Rotace: starý token `MarkUsed`, ne smazán (pro theft detection)
  - Theft detection: použití použitého tokenu → `DeleteByUserID` (force logout na všech zařízeních)
  - Test: success / expired / unknown / user-deleted / reuse-triggers-force-logout
- [x] **Task 4** — `application/auth/command/logout.go` + handler (Permissioned `auth:logout`): claims → `DeleteByUserID`
  - Test: deletes-all / does-not-touch-other-users / no-claims-returns-auth-error / required-permission
- [x] **Task 5** — `application/profile/command/change_password.go` + handler (Permissioned `profile:update`): claims → FindByID → Verify old → Hash new → Update
  - Test: success / wrong-old-password / no-claims / unknown-user / required-permission
  - **Refactor:** přesunul `testfx` z `auth/command/internal/` do `application/internal/` (sdílené napříč subpackages)
- [x] **Task 6** — `application/profile/query/get_profile.go` + handler (Permissioned `profile:read`): claims.UserID → `FindByID`
  - Test: success / no-claims / unknown-user / required-permission
- [x] **Task 7** — `domain/shared/permissions_registry.go` — `NewPermissionsRegistry([]Permissioned)` → `All()` / `ForRole(role) []string`
  - Shared helper `IsPermissionAllowedForRole(permission, role)` v `domain/shared/permission.go`, `PermissionChecker` ho teď používá (DRY)
  - Test: All sorted+dedup / ForRole(admin) = vše / ForRole(user) = bez `admin:*` / unknown-role = jako user / empty / copy-safety / 8 sub-testů pro helper
- [x] **Task 8** — `presentation/http/middleware/auth.go` — parse Bearer → `JwtService.Validate` → `ContextWithClaims`
  - **Error refactor:** `AuthError` 403→401, přidán `PermissionError` 403 (401 = nejsi přihlášen, 403 = nemáš právo)
  - **Fixture přesun:** `application/internal/testfx` → `app/internal/testfx` (sdílené přes všechny `app/**` vrstvy)
  - Test: valid-sets-claims / no-header-passes-through / missing-bearer-prefix-401 / invalid-token-401 / expired-401 / empty-bearer-401
- [x] **Task 9** — `presentation/http/handler/auth.go` — POST login / refresh / logout přes bus
  - `config.CookieSecure` (env `APP_COOKIE_SECURE`, default true, dev false)
  - `testfx.NewBuses()` helper — produkčně nakonfigurovaný stack busů
  - Arch-lint: `deepScan: false` (false positives v DI wiringu v testfx)
  - 8 E2E testů (real bus + real SQLite): login success / invalid creds / malformed JSON / refresh valid cookie / refresh missing cookie / refresh invalid cookie clears / logout no-claims 401 / logout with claims 204
- [x] **Task 10** — ProfileHandler (Get, ChangePassword) + server registrace všech rout
  - `presentation/http/handler/profile.go` — Get vrací userDTO s permissions, ChangePassword 204
  - `server.go` — public routy + protected (JWT AuthMiddleware wrap) + SPA fallback
  - 7 E2E testů (get success user / get success admin / get no-claims 401 / change success / change wrong-old 401 / change malformed 400 / change no-claims 401)
- [x] **Task 11** — Wire DI: všechny auth/profile handlery, `CookieSecure` typed flag, `PermissionsRegistry` provider, `shared.JwtService` binding
  - `make dev` sestaví binárku, `./bin/app serve` nastartuje, curl smoke test login→profile→logout prošel end-to-end
- [x] **Task 12** — Permissions v response (už hotové v Task 9/10: `registry.ForRole(role)` přímo v HTTP handleru)

### Frontend

- [x] **Task 13** — 401 auto-refresh + retry
  - Refactor Fetch/Auth na jednosměrné vrstvy (Fetch → Auth → Views), odstraněn auth bridge
  - `Fetch/`: 6 single-purpose souborů (apiFetch, apiUpload, apiDownload, accessToken, buildHeaders, parseResponse) + index
  - `Auth/`: 7 single-purpose souborů (state, login, logout, refresh, permissions, authFetch, useAuth) + index
  - `authFetch` = apiFetch + 401 retry + single-flight coalescing; skip pro `/api/v1/auth/*`
  - Testy: `tests/fetch/apiFetch.test.ts` (7) + `tests/auth/authFetch.test.ts` (5 integration, mock jen fetch)
- [x] **Task 14** — `router.ts` — `authGuard` + povinné `meta.requiresAuth` (mirror backendu Permissioned/SkipPermission)
  - `AppRoute` type forces explicit `meta.requiresAuth: true|false` — TS nepustí route bez deklarace
  - Runtime fail-closed pro bypass (missing meta → redirect /home + error toast)
  - Redirect na `/login` s `?redirect=<path>`, toasty (info / error), admin shortcut přes `hasPermission`
  - Stubs: `LoginView.vue`, `ProfileView.vue`, `AdminUsersView.vue` (Task 15/16/25 rozšíří)
  - Production routes: `/` + `/login` (public), `/profile` (auth), `/admin/users` (auth + permission)
  - 8 testů v `tests/router/authGuard.test.ts` (memory history + isolated state)
- [x] **Task 15** — `app/Auth/Views/LoginView.vue` — form nickname + password, volá `login()`
  - Error state: zpráva z response do `Input` error slotu + error toast
  - Redirect: `?redirect` query (z guardu) nebo `/`
  - Success toast "Vítej zpátky, {nickname}"
  - Loading state: disabled form + spinner v tlačítku
- [ ] **Task 16** — `app/Profile/Views/ProfileView.vue` — user data + "změnit heslo" form

---

## Fáze 7: Admin CRUD

- [ ] **Task 17** — `application/user/command/create_user.go` + handler (Permissioned `admin:users:create`): Validate → Hash → Save → Collect `UserCreated`
  - Test: real sqlite + collector → user exists + event collected
- [ ] **Task 18** — `application/user/command/update_user.go` + handler
  - Test: seed + update → changes persist
- [ ] **Task 19** — `application/user/command/delete_user.go` + handler — refuse self-delete
  - Test: seed + delete → 0 users / self-delete → ValidationError
- [ ] **Task 20** — `application/user/query/list_users.go` + handler
  - Test: seed 3 → expect 3 sorted
- [ ] **Task 21** — `application/user/event/send_welcome_email.go` — subscribe na `UserCreated` (zatím jen log)
  - Test: dispatch event → log called
- [ ] **Task 22** — `presentation/http/middleware/role.go` — role guard (admin)
  - Test: httptest → claims.Role=admin → 200, user → 403
- [ ] **Task 23** — `presentation/http/handler/admin_users.go` — CRUD přes bus
  - Test: httptest s real stack
- [ ] **Task 24** — Server registrace admin rout
- [ ] **Task 25** — `app/Admin/Views/AdminUsersView.vue` — seznam + modal create/edit/delete
- [ ] **Task 26** — `app/Home/Views/DashboardView.vue` — post-login stránka

---

## Ostatní

- [ ] **Task 27** — `presentation/http/middleware/security.go` — security headers (HSTS, CSP, X-Frame-Options, Referrer-Policy, Permissions-Policy)
  - Test: httptest → všechny hlavičky přítomny
  - Cíl: A+ rating na securityheaders.com
- [ ] **Task 28** — `Makefile: build-all` — cross-compile matrix (linux/amd64, darwin/arm64, windows/amd64)
- [ ] **Task 29** — `docker/release/Dockerfile` + `docker-compose.yml`
  - Smoke test: `docker compose up` → `/health` 200

---

## Fáze: Fronty a scheduled tasks

**Problém:** EventBus je in-memory + synchronní po commit. Crash mezi commitem a dispatchem = ztracený event. Žádný retry, žádné delayed/recurring joby, žádná visibility.

### Scheduled tasks (cron-like)

- [ ] **Task 30** — `infrastructure/scheduler/scheduler.go` — `Every(interval, name, fn)` + graceful shutdown
  - Startuje ze serveru, běží v goroutine, respektuje `ctx.Done()`
  - Test: schedule fn → sleep → assert counter incremented
- [ ] **Task 31** — Zaregistrovat cleanup: `scheduler.Every(1*time.Hour, "cleanup:expired-tokens", tokens.DeleteExpired)`
  - Řeší narůstání `refresh_tokens` (po expiraci se smažou, použité tokeny i okno theft detection)

### Persistent job queue (SQLite)

- [ ] **Task 32** — Migrace: `jobs` tabulka (id, kind, payload JSON, run_at, attempts, max_attempts, locked_until, last_error, created_at, completed_at)
- [ ] **Task 33** — `domain/job/` — `Job` entity, `JobRepository` interface s `Enqueue`, `ClaimDue`, `Complete`, `Fail` metodami
- [ ] **Task 34** — `infrastructure/sqlite/job/` — implementace s atomickým claim pomocí `UPDATE ... RETURNING`
- [ ] **Task 35** — `application/job/` — `JobDispatcher` (interface pro enqueue z command handlerů), `JobHandlerRegistry` (mapování kind → fn)
- [ ] **Task 36** — `infrastructure/worker/worker.go` — goroutine worker, poll interval, concurrency, exponential backoff, context cancellation
  - Test: enqueue fn → worker zpracuje → complete / enqueue failing fn → retry až max_attempts
- [ ] **Task 37** — Integrace s eventy: `DispatchEventsMiddleware` může volitelně enqueue (pro těžké handlery jako email, external API)
  - Fast handlery (pure Go logika) zůstávají synchronní
  - Těžké handlery mají `JobPayload` interface + jsou registrované v `JobHandlerRegistry`

### CLI workflow

- [ ] **Task 38** — `./bin/app worker` — samostatný proces jen pro joby (pro horizontální škálování, `./bin/app serve` může worker i vypnout přes env)

---

## Progress

**Hotovo:** 12 / 38 tasků — celá Fáze 6 Backend ✓

**Další:** Task 13 — Frontend 401 retry v useFetch (+ router guards, LoginView, ProfileView)
