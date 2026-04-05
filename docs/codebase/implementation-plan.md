---
layout: 'page'
uri: '/codebase/implementation-plan'
position: 1
slug: 'codebase-implementation-plan'
parent: 'codebase'
navTitle: 'Plán implementace'
title: 'Plán implementace'
description: 'Detailní checklist implementace – soubor po souboru, fáze po fázi.'
---

# Plán implementace

Každá fáze končí spustitelnou aplikací. Wire DI se rozšiřuje iterativně s každou fází.


## Fáze 1: Skeleton – app startuje

Po této fázi: `./bin/app serve` spustí HTTP server s health check endpointem.

- [x] `go mod init` + `go get` – cobra, wire, godotenv, uuid, ncruces/go-sqlite3, sqlx, goose, golang-jwt/jwt, x/crypto, testify
- [x] `.gitignore`, `.env.example`
- [x] `app/config/config.go` – `Config` struct, `LoadConfig()` přes godotenv
- [x] `app/http/response/response.go` – `JSON()`, `Error()`, `HTTPError` interface, `HandleError()`
- [x] `app/http/http_middleware/trace.go` – trace ID generování, `TraceIDFromContext()`
- [x] `app/http/http_middleware/cors.go` – CORS z `APP_CORS_ORIGIN`
- [x] `app/http/http_middleware/logging.go` – HTTP request logging s trace ID (slog)
- [x] `app/http/handler/health.go` – `GET /health` handler
- [x] `app/http/server/server.go` – `Server` struct, `http.ServeMux`, CSRF (`http.CrossOriginProtection`), middleware chain, `Start()`
- [x] `app/console/root.go` – Cobra root command
- [x] `app/console/serve.go` – `ServeCommand` spustí server
- [x] `app/application.go` – `Application` struct s `Run()`
- [x] `cmd/main.go` – entry point, slog logger setup, volá `di.CreateApplication()`
- [x] `app/di/container_provider.go` – Wire: Config, Server, ServeCommand, RootCommand, Application
- [x] `Makefile` – `install`, `dev`, `build`, `serve`, `di`, `install-tools`
- [x] Ověřit: `make dev && make serve` → health check na `localhost:3000/health`


## Fáze 2: Databáze – migrace běží

Po této fázi: server při startu automaticky migruje SQLite databázi.

- [x] `app/database/sqlite_manager.go` – `SqliteManager`, `NewSqliteManager()`, `ContextWithTx()`, `TxFromContext()`
- [x] `app/database/migration_manager.go` – `MigrationManager`, `RunUp()` přes embedded Goose migrace
- [x] `migrations/embed.go` – `//go:embed *.sql`
- [x] `migrations/20260327000001_create_users_table.sql` – users (id, nickname UNIQUE, email NOT NULL UNIQUE, password_hash, role CHECK, active, timestamps)
- [x] `migrations/20260327000002_create_refresh_tokens_table.sql` – refresh_tokens (id, user_id FK CASCADE, token_hash UNIQUE, expires_at, timestamps) + indexy
- [x] Wire: přidat `SqliteManager`, `MigrationManager` – migrace se spouští při startu
- [x] `Makefile` – `migrate-create`, `migrate-up`, `migrate-down`, `migrate-status`
- [x] Ověřit: server start → DB soubor vytvořen, tabulky existují


## Fáze 3: Domain – čisté jádro

Po této fázi: domain balíčky kompilují bez závislostí na ostatních balíčcích.

- [x] `app/domain/shared/errors.go` – `ValidationError` (400), `AuthError` (403), oba s `HTTPStatus()`
- [x] `app/domain/shared/auth_context.go` – `AuthClaims` struct, `ClaimsFromContext()`, `ContextWithClaims()`
- [x] `app/domain/shared/password.go` – `PasswordHasher` interface (Hash, Verify)
- [x] `app/domain/shared/permission.go` – `Permissioned`, `SkipPermission`, `PermissionChecker` interfaces
- [x] `app/domain/shared/event.go` – `DomainEvent` interface, `EventCollector` (Collect, Flush)
- [x] `app/domain/user/user_entity.go` – `User` entity, `NewUser()`
- [x] `app/domain/user/user_nickname.go` – `Nickname` value object
- [x] `app/domain/user/user_role.go` – `Role` value object
- [x] `app/domain/user/user_repository.go` – `Repository` interface
- [x] `app/domain/user/user_created_event.go` – `UserCreated` event
- [x] `app/domain/token/token_entity.go` – `RefreshToken` entity
- [x] `app/domain/token/token_repository.go` – `TokenRepository` interface
- [x] Ověřit: `go build ./app/domain/...` – shared→0 deps, user→shared, token→0 deps


## Fáze 4: Bus – middleware chain

Po této fázi: bus middleware chain funguje (zatím bez reálných handlerů).

- [x] `app/bus/bus.go` – `Bus` struct, `Middleware` type, `New()`, `execute()`
- [x] `app/bus/bus_exec.go` – `Exec[R]()` generická funkce
- [x] `app/bus/bus_void.go` – `ExecVoid()` zkratka
- [x] `app/bus/bus_types.go` – `CommandBus`, `QueryBus`, `EventBus` wrapper typy (Wire)
- [x] `app/bus/bus_middleware/middleware_recovery.go` – panic recovery, slog error
- [x] `app/bus/bus_middleware/middleware_logging.go` – slog s trace ID, command name, trvání
- [x] `app/bus/bus_middleware/middleware_authorize.go` – Permissioned / SkipPermission / default → error
- [x] `app/bus/bus_middleware/middleware_transaction.go` – BEGIN/COMMIT/ROLLBACK přes context
- [x] `app/bus/bus_middleware/middleware_events.go` – flush EventCollector po commitu
- [x] Wire: provider funkce připraveny, aktivují se ve fázi 6
- [x] Ověřit: bus balíček kompiluje, app startuje


## Fáze 5: Security + repozitáře

Po této fázi: security služby a SQLite repozitáře jsou připravené pro auth.

- [ ] `app/security/security_password.go` – bcrypt Hash/Verify, implementuje `shared.PasswordHasher`
- [ ] `app/security/security_jwt.go` – `JwtService`: `GenerateAccessToken()`, `ValidateAccessToken()`, `GenerateRefreshToken()` (`crypto/rand.Text()`)
- [ ] `app/security/security_permission.go` – implementuje `shared.PermissionChecker`, čte `AuthClaims` z contextu
- [ ] `app/sqlite/sqlite_user_repository.go` – `UserRepository`, implementuje `user.Repository` (7 metod), transakce přes `TxFromContext`
- [ ] `app/sqlite/sqlite_token_repository.go` – `TokenRepository`, implementuje `token.TokenRepository` (4 metody)
- [ ] Wire: `PasswordService` → `shared.PasswordHasher`, `JwtService`, `PermissionChecker` → `shared.PermissionChecker`, repozitáře → domain interfaces
- [ ] Wire: aktivovat bus providers (CommandBus s AuthorizeMiddleware, QueryBus, EventBus)
- [ ] Seed: výchozí admin uživatel (nickname + email + heslo z `.env` nebo hardcoded)
- [ ] Ověřit: seed vytvoří admin usera v DB, bus providers ve Wire


## Fáze 6: Auth – přihlášení funguje

Po této fázi: kompletní auth flow – login, refresh, logout, profil.

- [ ] `app/command/command_login.go` – `LoginCommand` (`SkipPermission`), `LoginHandler`, vrací `LoginResult`
- [ ] `app/command/command_refresh_token.go` – `RefreshTokenCommand` (`SkipPermission`), `RefreshTokenHandler`
- [ ] `app/command/command_logout.go` – `LogoutCommand` (`Permissioned: auth.logout`), `LogoutHandler`
- [ ] `app/command/command_change_password.go` – `ChangePasswordCommand` (`Permissioned: profile.update`), `ChangePasswordHandler`
- [ ] `app/query/query_get_profile.go` – `GetProfileQuery` (`Permissioned: profile.read`), `GetProfileHandler`
- [ ] `app/http/http_middleware/auth.go` – JWT Bearer middleware: token → `shared.ContextWithClaims()`
- [ ] `app/http/handler/handler_auth_login.go` – POST `/api/v1/auth/login`, set refresh cookie
- [ ] `app/http/handler/handler_auth_refresh.go` – POST `/api/v1/auth/refresh`, čte cookie
- [ ] `app/http/handler/handler_auth_logout.go` – POST `/api/v1/auth/logout`
- [ ] `app/http/handler/handler_profile.go` – GET `/api/v1/profile`, PUT `/api/v1/profile/password`
- [ ] Registrace rout v `server.go`: login+refresh (public), logout+profile (JWT auth)
- [ ] Wire: auth command/query handlery, HTTP handlery, bus providers
- [ ] Ověřit: curl login → access token + refresh cookie → curl profile s Bearer → 200


## Fáze 7: User management – admin CRUD

Po této fázi: admin může spravovat uživatele přes API.

- [ ] `app/command/command_create_user.go` – `CreateUserCommand` (`Permissioned: admin.users.create`), handler s validací + `UserCreated` event
- [ ] `app/command/command_update_user.go` – `UpdateUserCommand` (`Permissioned: admin.users.update`)
- [ ] `app/command/command_delete_user.go` – `DeleteUserCommand` (`Permissioned: admin.users.delete`), nemůže smazat sám sebe
- [ ] `app/query/query_list_users.go` – `ListUsersQuery` (`Permissioned: admin.users.read`)
- [ ] `app/http/handler/handler_admin_users.go` – `AdminUsersHandler` (Create, List, Update, Delete) přes bus
- [ ] `app/event/event_send_welcome_email.go` – `SendWelcomeEmailHandler` pro `UserCreated`
- [ ] Event handler registry v Wire
- [ ] Registrace admin rout v `server.go` s JWT auth + role guard
- [ ] Wire: admin command/query handlery, HTTP handler
- [ ] Ověřit: curl jako admin → CRUD, curl jako user → 403


## Fáze 8: Frontend – SPA

Po této fázi: Vue SPA s loginem, profilem a admin rozhraním.

- [ ] `package.json` – vue, vue-router, vite, tailwindcss, @tailwindcss/vite, typescript, vue-tsc, eslint, oxlint
- [ ] `index.html`, `vite.config.ts`, `tsconfig.json`, `env.d.ts`
- [ ] `public/embed.go` – `//go:embed *`
- [ ] `app/http/handler/handler_spa_fallback.go` – SPA fallback (embed.FS → index.html)
- [ ] Registrace SPA fallback v `server.go` jako catch-all `GET /{path...}`
- [ ] `assets/app.ts` – Vue mount s routerem
- [ ] `assets/tailwind.css`
- [ ] `assets/vue/App.vue` – root komponenta, layout
- [ ] `assets/vue/router/index.ts` – routes + guards (requiresAuth, requiredRole)
- [ ] `assets/vue/types/index.ts` – TypeScript typy
- [ ] `assets/vue/composables/useAuth.ts` – login, logout, refresh, scheduleRefresh
- [ ] `assets/vue/services/apiFetch.ts` – Authorization header, auto-refresh na 401
- [ ] `assets/vue/views/LoginView.vue` – nickname + heslo
- [ ] `assets/vue/views/DashboardView.vue` – chráněná stránka
- [ ] `assets/vue/views/ProfileView.vue` – profil + změna hesla
- [ ] `assets/vue/views/AdminUsersView.vue` – CRUD uživatelů
- [ ] `Makefile` – `fe-deps`, `fe-build`, `fe-dev`, `fe-clean`
- [ ] Wire: SPA fallback handler
- [ ] Ověřit: `make fe-dev` → login → dashboard → admin → profile


## Fáze 9: Build pipeline

Po této fázi: `make build` vytvoří single-binary s embedovaným frontendem.

- [ ] `Makefile` – `build` (di → fe-build → go-build), `build-all` (linux/amd64, darwin/arm64, windows/amd64)
- [ ] Ověřit: `make build` → `./bin/app serve` → SPA + API z jedné binárky
- [ ] E2E test: login → API call → token refresh → logout


## Fáze 10: Architecture enforcement

Po této fázi: `make arch-check` ověřuje dependency pravidla.

- [ ] `.go-arch-lint.yml` – komponenty: domain/shared, domain/user, domain/token, bus, bus/bus_middleware, command, query, event, sqlite, security, http/handler, http/http_middleware, http/server, http/response, config, database, console, di
- [ ] `Makefile` – `arch-check`
- [ ] Ověřit: `make arch-check` → zelené
- [ ] Záměrně porušit pravidlo → ověřit, že selže


## Fáze 11: Docker

Po této fázi: aplikace běží v Docker containeru.

- [ ] `docker/release/Dockerfile` – Alpine + binárka + SQLite volume
- [ ] `docker-compose.yml` – service s volume pro DB
- [ ] `make build-all` → linux binary
- [ ] `docker build` → `docker compose up -d`
- [ ] Ověřit: health check, login, API z containeru


## Fáze 12: Kvalita kódu

Po této fázi: kompletní CI pipeline projde.

- [ ] `eslint.config.ts` – ESLint + oxlint
- [ ] `.golangci.yml` – golangci-lint
- [ ] `go fix ./...` – Go 1.26 modernizace
- [ ] `Makefile` – `lint`, `lint-check`, `test`, `check` (lint-check + fe-type-check + test + arch-check)
- [ ] Ověřit: `make check` → zelené
