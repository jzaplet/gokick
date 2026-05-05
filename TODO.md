# TODO

Phases 1–5 are done. Phase 10 (arch-lint) was pulled forward and is also done.
DDD restructuring complete (domain/application/infrastructure/presentation).

## Overview

| Phase | What's left | Effort |
|---|---|---|
| **6: Auth** | BE: 5 command/query handlers, JWT middleware, HTTP handlers, routes, Wire, permissions in response. FE: router guards, 401 auto-refresh, LoginView, ProfileView | large |
| **7: Admin CRUD** | BE: create/update/delete/list user, role guard, UserCreated event. FE: AdminUsersView, DashboardView | medium |
| **9: Build pipeline** | `build-all` cross-compile, E2E test | small |
| **11: Docker** | Release Dockerfile, docker-compose app service | small |
| **Security Headers** | Middleware for A+ rating (HSTS, CSP, X-Frame-Options, Referrer-Policy, Permissions-Policy) | small |
| **Observability** | Sentry, slog attrs, OpenTelemetry | optional |
| **Documentation** | Sentry/OTel on the home page (once they exist) | minimal |

## ~~Phase 5: Security + repositories~~ ✓

Done — PasswordHasher, JwtService, PermissionChecker, repositories (sqlite/user, sqlite/token), seeder, Wire binding, 24 unit tests.

## Phase 6: Auth — login working

### Backend

- [ ] `app/application/auth/command/login.go` – LoginCommand (SkipPermission), returns LoginResult (access token + user + permissions)
- [ ] `app/application/auth/command/refresh_token.go` – RefreshTokenCommand (SkipPermission)
- [ ] `app/application/auth/command/logout.go` – LogoutCommand (Permissioned: auth.logout)
- [ ] `app/application/profile/command/change_password.go` – ChangePasswordCommand (Permissioned: profile.update)
- [ ] `app/application/profile/query/get_profile.go` – GetProfileQuery (Permissioned: profile.read)
- [ ] `app/presentation/http/middleware/auth.go` – JWT Bearer middleware: token → `shared.ContextWithClaims()`
- [ ] `app/presentation/http/handler/auth.go` – POST login (set refresh cookie), POST refresh (reads cookie), POST logout
- [ ] `app/presentation/http/handler/profile.go` – GET profile, PUT password
- [ ] `app/presentation/http/server/server.go` – route registration: login+refresh (public), logout+profile (JWT auth)
- [ ] `app/infrastructure/di/container_provider.go` – Wire: command/query handlers, HTTP handlers
- [ ] LoginResult must return `permissions: string[]` — assemble from every command/query handler implementing `shared.Permissioned` (RequiredPermission()) and filter by role
- [ ] GET /api/v1/auth/profile must return `permissions: string[]` (same logic)

### Frontend

- [ ] Router guards – requiresAuth (redirect to login + toast), requiresRole (403 toast + redirect)
- [ ] Auto-refresh on 401 in useFetch (retry after refresh token, logout on failure)
- [ ] `assets/app/Auth/Views/LoginView.vue`
- [ ] `assets/app/Profile/Views/ProfileView.vue`

### Verification

- [ ] curl login → access token + refresh cookie → curl profile with Bearer → 200
- [ ] Frontend: login → redirect → profile → logout → redirect to login

## Phase 7: User management — admin CRUD

### Backend

- [ ] `app/application/user/command/create_user.go` – with validation + UserCreated event (Permissioned: admin.users.create)
- [ ] `app/application/user/command/update_user.go` – (Permissioned: admin.users.update)
- [ ] `app/application/user/command/delete_user.go` – cannot delete self (Permissioned: admin.users.delete)
- [ ] `app/application/user/query/list_users.go` – (Permissioned: admin.users.read)
- [ ] `app/presentation/http/handler/admin_users.go` – CRUD via bus
- [ ] `app/presentation/http/middleware/role.go` – role guard middleware (admin)
- [ ] `app/application/user/event/send_welcome_email.go` – handler for UserCreated
- [ ] `app/presentation/http/server/server.go` – register admin routes with JWT auth + role guard
- [ ] `app/infrastructure/di/container_provider.go` – Wire: command/query handlers, HTTP handler, event handler registry

### Frontend

- [ ] `assets/app/Admin/Views/AdminUsersView.vue` – list, create, edit, delete
- [ ] `assets/app/Home/Views/DashboardView.vue` – landing page after login

## ~~Phase 8: Frontend — Vue 3 SPA~~ ✓

Done — Vue 3, Vue Router, Vite, Tailwind v4, TypeScript (maximum strictness), ESLint (strictTypeChecked + Stylistic), Vitest, Yarn v4, SPA embed + fallback handler.

- [x] `package.json` – vue, vue-router, vite, tailwindcss, typescript, eslint, vitest
- [x] `index.html`, `vite.config.ts`, `tsconfig.json`, `eslint.config.ts`, `env.d.ts`
- [x] `public/embed.go` – `//go:embed *`
- [x] `app/presentation/http/handler/spa.go` – SPA fallback (embed.FS → index.html)
- [x] `assets/app.ts`, `App.vue`, `router.ts`, `tailwind.css`
- [x] `app-ui/Auth/useAuth.ts` – login, logout, refresh, hasRole, hasPermission, isAdmin
- [x] `app-ui/Fetch/useFetch.ts` – apiFetch, apiUpload, apiDownload
- [x] `app-ui/Toast/useToast.ts` – success, error, info, warning, clear
- [x] `app-ui/` – Buttons, Icons, Inputs, Loading, Modals
- [x] `tests/` – Vitest + Vue Test Utils
- [x] Makefile: fe-deps, fe-build, fe-dev, fe-clean
- [x] Wire: SPA fallback handler

## ~~Phase 9: Build pipeline~~ ✓ (basics)

- [x] Makefile: build (di → fe-build → go-build), lint, format, test
- [ ] Makefile: build-all (linux/amd64, darwin/arm64, windows/amd64)
- [ ] E2E test: login → API → refresh → logout

## Phase 11: Docker

- [ ] `docker/release/Dockerfile` – Alpine + single binary + SQLite volume
- [ ] `docker-compose.yml` – app service with a volume for the DB
- [ ] `make build-all` → linux binary → `docker build` → `docker compose up -d`
- [ ] Verify: health check + login from inside the container

## ~~Phase 12: Code quality~~ ✓

Done — ESLint (strictTypeChecked + Vue recommended + Stylistic), golangci-lint, go-arch-lint, vue-tsc, Vitest, golines, documan-lint.

- [x] `eslint.config.ts` – ESLint + typescript-eslint + eslint-plugin-vue + @stylistic/eslint-plugin
- [x] golangci-lint + go-arch-lint
- [x] Makefile: lint (ESLint + vue-tsc + golangci-lint + go-arch-lint + documan-lint)
- [x] Makefile: format (ESLint Stylistic + golines + documan-fix)
- [x] Makefile: test (vitest + go test app/ + cmd/)

## Security Headers (A+ rating)

- [ ] `app/presentation/http/middleware/security.go` – security headers middleware
- [ ] `Strict-Transport-Security: max-age=31536000; includeSubDomains; preload` (HSTS)
- [ ] `Content-Security-Policy` – strict CSP (script-src, style-src, img-src, connect-src)
- [ ] `X-Content-Type-Options: nosniff`
- [ ] `X-Frame-Options: DENY`
- [ ] `Referrer-Policy: strict-origin-when-cross-origin`
- [ ] `Permissions-Policy` – disable camera, microphone, geolocation, etc.
- [ ] `X-XSS-Protection: 0` (deprecated, but for older browsers)
- [ ] Register in server.go middleware chain (before CORS)
- [ ] Verify: securityheaders.com → A+ rating

## Observability

- [ ] Sentry integration (SENTRY_DSN env, recovery middleware → Sentry report)
- [ ] Structured slog attrs (request_id, user_id, command) – verify consistency
- [ ] OpenTelemetry – optional, if distributed tracing is needed

## Documentation

- [x] Guides: Authentication, Frontend Utils
- [x] Dev Stack: current frontend stack + structure
- [x] Installation: prerequisites + make commands
- [ ] Home page – add Sentry and OpenTelemetry to the features list (once implemented)
