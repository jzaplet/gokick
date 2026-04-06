# TODO

Fáze 1–5 hotové. Fáze 10 (arch-lint) předřazena a hotová.
DDD restrukturalizace dokončena (domain/application/infrastructure/presentation).

## ~~Fáze 5: Security + repozitáře~~ ✓

Hotovo – PasswordHasher, JwtService, PermissionChecker, repozitáře (sqlite/user, sqlite/token), seeder, Wire binding, 24 unit testů.

## Fáze 6: Auth – přihlášení funguje

- [ ] `application/command/command_login.go` – LoginCommand (SkipPermission), vrací LoginResult (access token + user)
- [ ] `application/command/command_refresh_token.go` – RefreshTokenCommand (SkipPermission)
- [ ] `application/command/command_logout.go` – LogoutCommand (Permissioned: auth.logout)
- [ ] `application/command/command_change_password.go` – ChangePasswordCommand (Permissioned: profile.update)
- [ ] `application/query/query_get_profile.go` – GetProfileQuery (Permissioned: profile.read)
- [ ] `presentation/http/middleware/auth.go` – JWT Bearer middleware: token → `shared.ContextWithClaims()`
- [ ] `presentation/http/handler/handler_auth.go` – POST login (set refresh cookie), POST refresh (čte cookie), POST logout
- [ ] `presentation/http/handler/handler_profile.go` – GET profile, PUT password
- [ ] Registrace rout v server.go: login+refresh (public), logout+profile (JWT auth)
- [ ] Wire: command/query handlery, HTTP handlery, bus providers
- [ ] Ověřit: curl login → access token + refresh cookie → curl profile s Bearer → 200
- [ ] Backend: LoginResult musí vracet `permissions: string[]` — sestavit ze všech command/query handlerů implementujících `shared.Permissioned` (RequiredPermission()) a vyfiltrovat podle role
- [ ] Backend: GET /api/v1/auth/profile musí vracet `permissions: string[]` (stejná logika)

## Fáze 7: User management – admin CRUD

- [ ] `application/command/command_create_user.go` – s validací + UserCreated event (Permissioned: admin.users.create)
- [ ] `application/command/command_update_user.go` – (Permissioned: admin.users.update)
- [ ] `application/command/command_delete_user.go` – nemůže smazat sám sebe (Permissioned: admin.users.delete)
- [ ] `application/query/query_list_users.go` – (Permissioned: admin.users.read)
- [ ] `presentation/http/handler/handler_admin_users.go` – CRUD přes bus
- [ ] `presentation/http/middleware/role.go` – role guard middleware (admin)
- [ ] `application/event/event_send_welcome_email.go` – handler pro UserCreated
- [ ] Registrace admin rout v server.go s JWT auth + role guard
- [ ] Wire: command/query handlery, HTTP handler, event handler registry

## ~~Fáze 8: Frontend – Vue 3 SPA~~ ✓ (scaffold)

Scaffold hotový – Vue 3, Vue Router, Vite, Tailwind v4, TypeScript (maximum strictness), ESLint (strictTypeChecked + Stylistic), Vitest, Yarn v4, SPA embed + fallback handler.

- [x] `package.json` – vue, vue-router, vite, tailwindcss, typescript, eslint
- [x] `index.html`, `vite.config.ts`, `tsconfig.json`, `env.d.ts`
- [x] `public/embed.go` – `//go:embed *`
- [x] `presentation/http/handler/handler_spa_fallback.go` – SPA fallback (embed.FS → index.html)
- [x] Registrace SPA fallback v server.go jako catch-all `GET /{path...}`
- [x] `assets/app.ts` – Vue mount s routerem
- [x] `assets/tailwind.css`
- [x] `assets/vue/App.vue` – root komponenta, layout
- [x] `assets/vue/types/router.ts` – TypeScript typy
- [x] `assets/vue/router/router.ts` – routes (guards přijdou s Fází 6)
- [x] `app-ui/Auth/useAuth.ts` – login, logout, refresh, scheduleRefresh, hasRole, hasPermission
- [x] `app-ui/Fetch/useFetch.ts` – apiFetch s generikou, apiUpload, Authorization header
- [ ] Router guards – requiresAuth (redirect na login + toast), requiresRole (403 toast + redirect)
- [ ] Auto-refresh na 401 v useFetch (retry po refresh tokenu, logout při selhání)
- [ ] `assets/app/Auth/Views/LoginView.vue`
- [ ] `assets/app/Home/Views/DashboardView.vue`
- [ ] `assets/app/Profile/Views/ProfileView.vue`
- [ ] `assets/app/Admin/Views/AdminUsersView.vue`
- [x] Makefile: fe-deps, fe-build, fe-dev, fe-clean
- [x] Wire: SPA fallback handler

## ~~Fáze 9: Build pipeline~~ ✓ (základ)

- [x] Makefile: build (di → fe-build → go-build)
- [ ] Makefile: build-all (linux/amd64, darwin/arm64, windows/amd64)
- [ ] Ověřit: `make build` → `./bin/app serve` → SPA + API z jedné binárky
- [ ] E2E test: login → API → refresh → logout

## Fáze 11: Docker

- [ ] `docker/release/Dockerfile` – Alpine + single binary + SQLite volume
- [ ] `docker-compose.yml` – app service s volume pro DB
- [ ] `make build-all` → linux binary → `docker build` → `docker compose up -d`
- [ ] Ověřit: health check + login z containeru

## ~~Fáze 12: Kvalita kódu~~ ✓

Hotovo – ESLint (strictTypeChecked + Vue recommended + Stylistic), golangci-lint, go-arch-lint, vue-tsc, Vitest, golines.

- [x] `eslint.config.ts` – ESLint + typescript-eslint + eslint-plugin-vue + @stylistic/eslint-plugin
- [x] golangci-lint – již nakonfigurovaný
- [x] Makefile: lint (ESLint + vue-tsc + golangci-lint + go-arch-lint), format (ESLint Stylistic + golines), test (vitest + go test)

## Observability

- [ ] Sentry integrace (SENTRY_DSN env, recovery middleware → Sentry report)
- [ ] Strukturované slog attrs (request_id, user_id, command) – ověřit konzistenci
- [ ] OpenTelemetry – volitelně, pokud bude potřeba distributed tracing

## Dokumentace

- [ ] Home stránka – dopsat FE komponenty (Vue views, composables, services) a návod jak číst dokumentaci
- [ ] Home stránka – dopsat Sentry a OpenTelemetry do vlastností (až budou implementované)
- [ ] `docs/guides/` předělat na praktické návody (Proč/Jak/Detaily) nebo smazat
