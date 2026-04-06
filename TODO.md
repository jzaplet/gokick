# TODO

Fáze 1–5 hotové. Fáze 10 (arch-lint) předřazena a hotová.
DDD restrukturalizace dokončena (domain/application/infrastructure/presentation).

## ~~Fáze 5: Security + repozitáře~~ ✓

Hotovo – PasswordHasher, JwtService, PermissionChecker, repozitáře (sqlite/user, sqlite/token), seeder, Wire binding, 24 unit testů.

## Fáze 6: Auth – přihlášení funguje

### Backend

- [ ] `app/application/auth/command/login.go` – LoginCommand (SkipPermission), vrací LoginResult (access token + user + permissions)
- [ ] `app/application/auth/command/refresh_token.go` – RefreshTokenCommand (SkipPermission)
- [ ] `app/application/auth/command/logout.go` – LogoutCommand (Permissioned: auth.logout)
- [ ] `app/application/profile/command/change_password.go` – ChangePasswordCommand (Permissioned: profile.update)
- [ ] `app/application/profile/query/get_profile.go` – GetProfileQuery (Permissioned: profile.read)
- [ ] `app/presentation/http/middleware/auth.go` – JWT Bearer middleware: token → `shared.ContextWithClaims()`
- [ ] `app/presentation/http/handler/auth.go` – POST login (set refresh cookie), POST refresh (čte cookie), POST logout
- [ ] `app/presentation/http/handler/profile.go` – GET profile, PUT password
- [ ] `app/presentation/http/server/server.go` – registrace rout: login+refresh (public), logout+profile (JWT auth)
- [ ] `app/infrastructure/di/container_provider.go` – Wire: command/query handlery, HTTP handlery
- [ ] LoginResult musí vracet `permissions: string[]` — sestavit ze všech command/query handlerů implementujících `shared.Permissioned` (RequiredPermission()) a vyfiltrovat podle role
- [ ] GET /api/v1/auth/profile musí vracet `permissions: string[]` (stejná logika)

### Frontend

- [ ] Router guards – requiresAuth (redirect na login + toast), requiresRole (403 toast + redirect)
- [ ] Auto-refresh na 401 v useFetch (retry po refresh tokenu, logout při selhání)
- [ ] `assets/app/Auth/Views/LoginView.vue`
- [ ] `assets/app/Profile/Views/ProfileView.vue`

### Ověření

- [ ] curl login → access token + refresh cookie → curl profile s Bearer → 200
- [ ] Frontend: login → redirect → profile → logout → redirect na login

## Fáze 7: User management – admin CRUD

### Backend

- [ ] `app/application/user/command/create_user.go` – s validací + UserCreated event (Permissioned: admin.users.create)
- [ ] `app/application/user/command/update_user.go` – (Permissioned: admin.users.update)
- [ ] `app/application/user/command/delete_user.go` – nemůže smazat sám sebe (Permissioned: admin.users.delete)
- [ ] `app/application/user/query/list_users.go` – (Permissioned: admin.users.read)
- [ ] `app/presentation/http/handler/admin_users.go` – CRUD přes bus
- [ ] `app/presentation/http/middleware/role.go` – role guard middleware (admin)
- [ ] `app/application/user/event/send_welcome_email.go` – handler pro UserCreated
- [ ] `app/presentation/http/server/server.go` – registrace admin rout s JWT auth + role guard
- [ ] `app/infrastructure/di/container_provider.go` – Wire: command/query handlery, HTTP handler, event handler registry

### Frontend

- [ ] `assets/app/Admin/Views/AdminUsersView.vue` – seznam, vytvoření, editace, smazání
- [ ] `assets/app/Home/Views/DashboardView.vue` – úvodní stránka po přihlášení

## ~~Fáze 8: Frontend – Vue 3 SPA~~ ✓

Hotovo – Vue 3, Vue Router, Vite, Tailwind v4, TypeScript (maximum strictness), ESLint (strictTypeChecked + Stylistic), Vitest, Yarn v4, SPA embed + fallback handler.

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

## ~~Fáze 9: Build pipeline~~ ✓ (základ)

- [x] Makefile: build (di → fe-build → go-build), lint, format, test
- [ ] Makefile: build-all (linux/amd64, darwin/arm64, windows/amd64)
- [ ] E2E test: login → API → refresh → logout

## Fáze 11: Docker

- [ ] `docker/release/Dockerfile` – Alpine + single binary + SQLite volume
- [ ] `docker-compose.yml` – app service s volume pro DB
- [ ] `make build-all` → linux binary → `docker build` → `docker compose up -d`
- [ ] Ověřit: health check + login z containeru

## ~~Fáze 12: Kvalita kódu~~ ✓

Hotovo – ESLint (strictTypeChecked + Vue recommended + Stylistic), golangci-lint, go-arch-lint, vue-tsc, Vitest, golines, documan-lint.

- [x] `eslint.config.ts` – ESLint + typescript-eslint + eslint-plugin-vue + @stylistic/eslint-plugin
- [x] golangci-lint + go-arch-lint
- [x] Makefile: lint (ESLint + vue-tsc + golangci-lint + go-arch-lint + documan-lint)
- [x] Makefile: format (ESLint Stylistic + golines + documan-fix)
- [x] Makefile: test (vitest + go test app/ + cmd/)

## Security Headers (A+ rating)

- [ ] `app/presentation/http/middleware/security.go` – Security headers middleware
- [ ] `Strict-Transport-Security: max-age=31536000; includeSubDomains; preload` (HSTS)
- [ ] `Content-Security-Policy` – strict CSP (script-src, style-src, img-src, connect-src)
- [ ] `X-Content-Type-Options: nosniff`
- [ ] `X-Frame-Options: DENY`
- [ ] `Referrer-Policy: strict-origin-when-cross-origin`
- [ ] `Permissions-Policy` – zakázat camera, microphone, geolocation, atd.
- [ ] `X-XSS-Protection: 0` (deprecated, ale pro starší prohlížeče)
- [ ] Registrace v server.go middleware chain (před CORS)
- [ ] Ověřit: securityheaders.com → A+ rating

## Observability

- [ ] Sentry integrace (SENTRY_DSN env, recovery middleware → Sentry report)
- [ ] Strukturované slog attrs (request_id, user_id, command) – ověřit konzistenci
- [ ] OpenTelemetry – volitelně, pokud bude potřeba distributed tracing

## Dokumentace

- [x] Guides: Authentication, Frontend Utils
- [x] Dev Stack: aktuální frontend stack + struktura
- [x] Installation: prerekvizity + make příkazy
- [ ] Home stránka – dopsat Sentry a OpenTelemetry do vlastností (až budou implementované)
