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

## Fáze 8: Frontend – Vue 3 SPA

- [ ] `package.json` – vue, vue-router, vite, tailwindcss, typescript, eslint, oxlint
- [ ] `index.html`, `vite.config.ts`, `tsconfig.json`, `env.d.ts`
- [ ] `public/embed.go` – `//go:embed *`
- [ ] `presentation/http/handler/handler_spa_fallback.go` – SPA fallback (embed.FS → index.html)
- [ ] Registrace SPA fallback v server.go jako catch-all `GET /{path...}`
- [ ] `assets/app.ts` – Vue mount s routerem
- [ ] `assets/tailwind.css`
- [ ] `assets/vue/App.vue` – root komponenta, layout
- [ ] `assets/vue/router/index.ts` – routes + guards (requiresAuth, requiredRole)
- [ ] `assets/vue/types/index.ts` – TypeScript typy
- [ ] `assets/vue/composables/useAuth.ts` – login, logout, refresh, scheduleRefresh
- [ ] `assets/vue/services/apiFetch.ts` – Authorization header, auto-refresh na 401
- [ ] `assets/vue/views/LoginView.vue`
- [ ] `assets/vue/views/DashboardView.vue`
- [ ] `assets/vue/views/ProfileView.vue`
- [ ] `assets/vue/views/AdminUsersView.vue`
- [ ] Makefile: fe-deps, fe-build, fe-dev, fe-clean
- [ ] Wire: SPA fallback handler

## Fáze 9: Build pipeline

- [ ] Makefile: build (di → fe-build → go-build), build-all (linux/amd64, darwin/arm64, windows/amd64)
- [ ] Ověřit: `make build` → `./bin/app serve` → SPA + API z jedné binárky
- [ ] E2E test: login → API → refresh → logout

## Fáze 11: Docker

- [ ] `docker/release/Dockerfile` – Alpine + single binary + SQLite volume
- [ ] `docker-compose.yml` – app service s volume pro DB
- [ ] `make build-all` → linux binary → `docker build` → `docker compose up -d`
- [ ] Ověřit: health check + login z containeru

## Fáze 12: Kvalita kódu

- [ ] `eslint.config.ts` – ESLint + oxlint
- [ ] `.golangci.yml` – golangci-lint
- [ ] `go fix ./...` (Go 1.26)
- [ ] Makefile: lint, lint-check, test, check (= lint-check + fe-type-check + test + arch-check)
- [ ] Ověřit: `make check` → zelené

## Observability

- [ ] Sentry integrace (SENTRY_DSN env, recovery middleware → Sentry report)
- [ ] Strukturované slog attrs (request_id, user_id, command) – ověřit konzistenci
- [ ] OpenTelemetry – volitelně, pokud bude potřeba distributed tracing

## Dokumentace

- [ ] Home stránka – dopsat FE komponenty (Vue views, composables, services) a návod jak číst dokumentaci
- [ ] Home stránka – dopsat Sentry a OpenTelemetry do vlastností (až budou implementované)
- [ ] `docs/guides/` předělat na praktické návody (Proč/Jak/Detaily) nebo smazat
