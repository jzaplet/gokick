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

- [ ] `go mod init` + `go get` – cobra, wire, godotenv, uuid, ncruces/go-sqlite3, sqlx, goose, golang-jwt/jwt, x/crypto, testify
- [ ] `.gitignore` – `bin/`, `data/`, `public/` (kromě `embed.go`), `.env`, `wire_gen.go`
- [ ] `.env.example` – `APP_HTTP_PORT=3000`, `APP_DB_PATH=./data/app.db`, `APP_JWT_SECRET`, `APP_JWT_ACCESS_EXPIRATION=15m`, `APP_JWT_REFRESH_EXPIRATION=168h`, `APP_CORS_ORIGIN=http://localhost:5173`
- [ ] `app/env/config.go` – `Config` struct, `LoadConfig()` přes godotenv
- [ ] `app/response/response.go` – `JSON()`, `Error()`, `HTTPError` interface, `HandleError()`
- [ ] `app/middleware/trace.go` – trace ID generování, `TraceIDFromContext()`
- [ ] `app/middleware/cors.go` – CORS z `APP_CORS_ORIGIN`
- [ ] `app/middleware/logging.go` – HTTP request logging s trace ID (slog)
- [ ] `app/handler/health.go` – `GET /health` handler
- [ ] `app/server/server.go` – `Server` struct, `http.ServeMux`, middleware registrace (Trace → CORS → CSRF → Logging), `Start()`
- [ ] CSRF – `http.CrossOriginProtection` (Go 1.25 stdlib) registrace v serveru
- [ ] `app/console/root.go` – Cobra root command
- [ ] `app/console/serve.go` – `ServeCommand` spustí server
- [ ] `app/application.go` – `Application` struct s `Run()`
- [ ] `app/main.go` – entry point, slog logger setup (JSON handler na stderr), volá `CreateApplication()`
- [ ] `app/di_container/container_provider.go` – Wire: Config, Server, ServeCommand, RootCommand, Application
- [ ] `make di` → ověřit `wire_gen.go`
- [ ] `Makefile` – `install`, `dev`, `serve`, `di`, `install-tools` (wire, golines, golangci-lint, goose, go-arch-lint)
- [ ] Ověřit: `make dev && make serve` → health check na `localhost:3000/health`


## Fáze 2: Databáze – migrace běží

Po této fázi: server při startu automaticky migruje SQLite databázi.

- [ ] `app/database/sqlite_manager.go` – `SqliteManager`, `NewSqliteManager()`, `ContextWithTx()`, `TxFromContext()`
- [ ] `app/database/migration_manager.go` – `MigrationManager`, `RunUp()` přes embedded Goose migrace
- [ ] `migrations/embed.go` – `//go:embed *.sql`
- [ ] `migrations/20260327000001_create_users_table.sql` – users (id, nickname UNIQUE, password_hash, email, role CHECK, active, timestamps)
- [ ] `migrations/20260327000002_create_refresh_tokens_table.sql` – refresh_tokens (id, user_id FK CASCADE, token_hash UNIQUE, expires_at, timestamps) + indexy
- [ ] Wire: přidat `SqliteManager`, `MigrationManager` – migrace se spouští při startu
- [ ] `Makefile` – `migrate-create`, `migrate-up`, `migrate-down`, `migrate-status`
- [ ] Ověřit: server start → DB soubor vytvořen, tabulky existují


## Fáze 3: Domain – čisté jádro

Po této fázi: domain balíček kompiluje bez závislostí na ostatních balíčcích.

- [ ] `app/domain/errors.go` – `ValidationError` (400), `AuthError` (403), oba s `HTTPStatus()`
- [ ] `app/domain/auth_context.go` – `AuthClaims` struct, `ClaimsFromContext()`, `ContextWithClaims()`
- [ ] `app/domain/nickname.go` – `Nickname` value object, `NewNickname()` (povinný, max 50 znaků)
- [ ] `app/domain/role.go` – `Role` value object, `RoleAdmin`, `RoleUser`, `NewRole()`
- [ ] `app/domain/password.go` – `PasswordHasher` interface (Hash, Verify)
- [ ] `app/domain/permission.go` – `Permissioned`, `SkipPermission`, `PermissionChecker` interfaces
- [ ] `app/domain/event.go` – `DomainEvent` interface, `EventCollector` (Collect, Flush)
- [ ] `app/domain/user.go` – `User` entity, `NewUser(Nickname, passwordHash, *string email, Role)`, `UserRepository` interface (Save, Update, Delete, FindByID, FindByNickname, FindAllActive, FindAll)
- [ ] `app/domain/auth.go` – `RefreshToken` entity, `TokenRepository` interface (Save, FindByHash, DeleteByUserID, DeleteExpired)
- [ ] `app/domain/events/user_created.go` – `UserCreated` event (UserID, Nickname, Email, Role, Timestamp)
- [ ] Ověřit: `go build ./app/domain/...` – kompiluje bez app/ importů


## Fáze 4: Bus – middleware chain

Po této fázi: bus middleware chain funguje (zatím bez reálných handlerů).

- [ ] `app/bus/bus.go` – `Bus` struct, `Middleware` type (ctx, name, cmd, next), `New()`
- [ ] `app/bus/exec.go` – `Exec[R]()` generická funkce s cmd parametrem
- [ ] `app/bus/void.go` – `ExecVoid()` zkratka
- [ ] `app/bus/middleware_recovery.go` – panic recovery, slog error + optional Sentry report
- [ ] `app/bus/middleware_logging.go` – slog s trace ID, command name, trvání, error
- [ ] `app/bus/middleware_authorize.go` – switch: `Permissioned` → check, `SkipPermission` → skip, default → error
- [ ] `app/bus/middleware_transaction.go` – BEGIN/COMMIT/ROLLBACK, tx do contextu
- [ ] `app/bus/middleware_events.go` – flush `EventCollector` po commitu, async goroutine dispatch
- [ ] Wire: `CommandBus` (Recovery → Logging → Authorize → Transaction → DispatchEvents), `QueryBus` (Recovery → Logging → Authorize), `EventBus` (Recovery → Logging), `EventCollector`
- [ ] Ověřit: Wire generuje, app startuje


## Fáze 5: Security + repozitáře – adaptéry

Po této fázi: security služby a SQLite repozitáře jsou připravené pro auth.

- [ ] `app/security/password_service.go` – bcrypt Hash/Verify, implementuje `domain.PasswordHasher`
- [ ] `app/security/jwt_service.go` – `GenerateAccessToken()` (HS256 JWT), `ValidateAccessToken()`, `GenerateRefreshToken()` (`crypto/rand.Text()`)
- [ ] `app/security/permission_checker.go` – implementuje `domain.PermissionChecker`, čte `AuthClaims` z contextu
- [ ] `app/sqlite/user_repository.go` – `SqliteUserRepository`, implementuje `domain.UserRepository` (7 metod), transakce přes `TxFromContext`
- [ ] `app/sqlite/token_repository.go` – `SqliteTokenRepository`, implementuje `domain.TokenRepository` (4 metody)
- [ ] Wire: `PasswordService` → `domain.PasswordHasher`, `JwtService`, `PermissionChecker` → `domain.PermissionChecker`, `SqliteUserRepository` → `domain.UserRepository`, `SqliteTokenRepository` → `domain.TokenRepository`
- [ ] Seed: výchozí admin uživatel (nickname + heslo z `.env` nebo hardcoded pro dev)
- [ ] Ověřit: seed vytvoří admin usera v DB


## Fáze 6: Auth – přihlášení funguje

Po této fázi: kompletní auth flow – login, refresh, logout, profil.

- [ ] `app/command/login.go` – `LoginCommand` (`SkipPermission`), `LoginHandler` (verify heslo, generuj access+refresh token, ulož hash do DB), vrací `LoginResult`
- [ ] `app/command/refresh_token.go` – `RefreshTokenCommand` (`SkipPermission`), `RefreshTokenHandler` (hash lookup, rotace, nový access token)
- [ ] `app/command/logout.go` – `LogoutCommand` (`Permissioned: auth.logout`), `LogoutHandler` (smaže refresh tokeny z DB)
- [ ] `app/command/change_password.go` – `ChangePasswordCommand` (`Permissioned: profile.update`), `ChangePasswordHandler` (verify old, hash new)
- [ ] `app/query/get_profile.go` – `GetProfileQuery` (`Permissioned: profile.read`), `GetProfileHandler`
- [ ] `app/middleware/auth.go` – JWT Bearer middleware: extrahuje token z hlavičky, validuje přes `JwtService`, claims do contextu přes `domain.ContextWithClaims()`
- [ ] `app/handler/auth_login.go` – POST `/api/v1/auth/login` (nickname + heslo), set refresh cookie (httpOnly, Secure, SameSite=Strict)
- [ ] `app/handler/auth_refresh.go` – POST `/api/v1/auth/refresh`, čte refresh token z cookie
- [ ] `app/handler/auth_logout.go` – POST `/api/v1/auth/logout`
- [ ] `app/handler/profile.go` – GET `/api/v1/profile`, PUT `/api/v1/profile/password`
- [ ] Registrace rout v `server.go`: login+refresh (public), logout+profile (JWT auth middleware)
- [ ] Wire: všechny auth command/query handlery, HTTP handlery
- [ ] Ověřit: curl login → access token + refresh cookie → curl profile s Bearer → 200


## Fáze 7: User management – admin CRUD

Po této fázi: admin může spravovat uživatele přes API.

- [ ] `app/command/create_user.go` – `CreateUserCommand` (`Permissioned: admin.users.create`), `CreateUserHandler` (value object validace, unique check, hash heslo, `UserCreated` event)
- [ ] `app/command/update_user.go` – `UpdateUserCommand` (`Permissioned: admin.users.update`), `UpdateUserHandler`
- [ ] `app/command/delete_user.go` – `DeleteUserCommand` (`Permissioned: admin.users.delete`), `DeleteUserHandler` (nemůže smazat sám sebe)
- [ ] `app/query/list_users.go` – `ListUsersQuery` (`Permissioned: admin.users.read`), `ListUsersHandler`
- [ ] `app/handler/admin_users.go` – `AdminUsersHandler` (HandleCreate, HandleList, HandleUpdate, HandleDelete) přes bus
- [ ] `app/event/send_welcome_email.go` – `SendWelcomeEmailHandler` pro `UserCreated` event
- [ ] Event handler registry v Wire: `"user.created"` → `sendWelcomeEmailHandler`
- [ ] Registrace admin rout v `server.go` s JWT auth + role guard middleware
- [ ] Wire: admin command/query handlery, HTTP handler
- [ ] Ověřit: curl jako admin → CRUD uživatelů, curl jako user → 403


## Fáze 8: Frontend – SPA

Po této fázi: Vue SPA s loginem, profilem a admin rozhraním.

- [ ] `package.json` – vue, vue-router, vite, tailwindcss, @tailwindcss/vite, typescript, vue-tsc, eslint, oxlint
- [ ] `index.html` – HTML entry point
- [ ] `vite.config.ts` – build do `public/`, proxy `/api` + `/health` → `localhost:3000`
- [ ] `tsconfig.json` + `env.d.ts`
- [ ] `public/embed.go` – `//go:embed *`
- [ ] `app/handler/spa_fallback.go` – SPA fallback handler (embed.FS, neznámé cesty → index.html)
- [ ] Registrace SPA fallback v `server.go` jako catch-all `GET /{path...}`
- [ ] `assets/app.ts` – Vue mount s routerem
- [ ] `assets/tailwind.css`
- [ ] `assets/vue/App.vue` – root komponenta, layout (navigace, status bar, logout)
- [ ] `assets/vue/router/index.ts` – routes: `/login`, `/` (dashboard), `/profile`, `/admin/users`, guards (requiresAuth, requiredRole)
- [ ] `assets/vue/types/index.ts` – TypeScript typy (AuthUser, LoginResponse, apod.)
- [ ] `assets/vue/composables/useAuth.ts` – login(), logout(), refresh(), scheduleRefresh(), isAuthenticated, currentUser
- [ ] `assets/vue/services/apiFetch.ts` – Authorization header, auto-refresh na 401, redirect na login při selhání
- [ ] `assets/vue/views/LoginView.vue` – nickname + heslo formulář
- [ ] `assets/vue/views/DashboardView.vue` – chráněná stránka (requiresAuth)
- [ ] `assets/vue/views/ProfileView.vue` – profil + formulář změny hesla
- [ ] `assets/vue/views/AdminUsersView.vue` – seznam + CRUD uživatelů (requiredRole: admin)
- [ ] `Makefile` – `fe-deps`, `fe-build`, `fe-dev`, `fe-clean`
- [ ] Wire: SPA fallback handler
- [ ] Ověřit: `make fe-dev` → Vite HMR, login → dashboard → admin → profile


## Fáze 9: Build pipeline

Po této fázi: `make build` vytvoří single-binary s embedovaným frontendem.

- [ ] `Makefile` – `build` (di → fe-build → go-build), `build-all` (linux/amd64, darwin/arm64, windows/amd64)
- [ ] Ověřit: `make build` → `./bin/app serve` → SPA + API funguje z jedné binárky
- [ ] E2E test: login → API call → token refresh → logout → redirect na login


## Fáze 10: Architecture enforcement

Po této fázi: `make arch-check` ověřuje dependency pravidla.

- [ ] `.go-arch-lint.yml` – 15 komponent (domain, command, query, event, bus, sqlite, security, handler, middleware, server, console, di_container, env, database, response), dependency pravidla per komponenta
- [ ] `Makefile` – `arch-check`
- [ ] Ověřit: `make arch-check` → zelené
- [ ] Záměrně porušit pravidlo (handler importuje sqlite) → ověřit, že arch-check selže


## Fáze 11: Docker

Po této fázi: aplikace běží v Docker containeru.

- [ ] `docker/release/Dockerfile` – Alpine, binárka, `APP_DB_PATH=/app/db/app.db`, `EXPOSE 3000`
- [ ] `docker-compose.yml` – service s volume pro SQLite DB
- [ ] `make build-all` → linux binary
- [ ] `docker build` → `docker compose up -d`
- [ ] Ověřit: health check, login, API funguje z containeru


## Fáze 12: Kvalita kódu

Po této fázi: kompletní CI pipeline projde.

- [ ] `eslint.config.ts` – ESLint + oxlint konfigurace
- [ ] `.golangci.yml` – golangci-lint konfigurace
- [ ] `go fix ./...` – Go 1.26 modernizace
- [ ] `Makefile` – `lint` (go-fmt + fe-lint + go-lint), `lint-check` (CI mód), `test`, `check` (lint-check + fe-type-check + test + arch-check)
- [ ] Ověřit: `make check` → zelené
