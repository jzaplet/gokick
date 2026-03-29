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


## Fáze 1: Projekt + konfigurace

- [ ] `go mod init` – Go modul
- [ ] `.env.example` – `APP_HTTP_PORT`, `APP_DB_PATH`, `APP_JWT_SECRET`, `APP_JWT_ACCESS_EXPIRATION`, `APP_JWT_REFRESH_EXPIRATION`, `APP_CORS_ORIGIN`
- [ ] `app/env/config.go` – `Config` struct, `LoadConfig()` přes `godotenv`
- [ ] `app/main.go` – entry point, volá `di_container.CreateApplication()`
- [ ] `app/application.go` – `Application` struct s `Run()`


## Fáze 2: CLI + HTTP server

- [ ] `app/console/root.go` – Cobra root command
- [ ] `app/console/serve.go` – `ServeCommand`, spustí server
- [ ] `app/server/server.go` – `Server` struct, `http.ServeMux`, `Start()`
- [ ] `GET /health` – health check handler
- [ ] `app/response/response.go` – `JSON()`, `Error()`, `HTTPError` interface, `HandleError()`
- [ ] `app/middleware/trace.go` – trace ID generování, `TraceIDFromContext()`
- [ ] `app/middleware/cors.go` – CORS z `APP_CORS_ORIGIN`
- [ ] `app/middleware/logging.go` – HTTP request logging s trace ID


## Fáze 3: Databáze + migrace

- [ ] `app/database/sqlite_manager.go` – `SqliteManager`, `ContextWithTx()`, `TxFromContext()`
- [ ] `app/database/migration_manager.go` – `MigrationManager`, `RunUp()`, auto-run při startu
- [ ] `migrations/embed.go` – `//go:embed *.sql`
- [ ] `migrations/20260327000001_create_users_table.sql` – `users` tabulka (id, nickname, password_hash, email, role, active, timestamps)
- [ ] `migrations/20260327000002_create_refresh_tokens_table.sql` – `refresh_tokens` tabulka (id, user_id, token_hash, expires_at, timestamps) + indexy


## Fáze 4: Domain

- [ ] `app/domain/errors.go` – `ValidationError` (400), `AuthError` (403), oba s `HTTPStatus()`
- [ ] `app/domain/auth_context.go` – `AuthClaims`, `ClaimsFromContext()`, `ContextWithClaims()`
- [ ] `app/domain/nickname.go` – `Nickname` value object, `NewNickname()` s validací
- [ ] `app/domain/role.go` – `Role` value object, `RoleAdmin`, `RoleUser`, `NewRole()`
- [ ] `app/domain/password.go` – `PasswordHasher` interface
- [ ] `app/domain/permission.go` – `Permissioned`, `SkipPermission`, `PermissionChecker` interfaces
- [ ] `app/domain/event.go` – `DomainEvent` interface, `EventCollector`
- [ ] `app/domain/user.go` – `User` entity, `NewUser()`, `UserRepository` interface
- [ ] `app/domain/auth.go` – `RefreshToken` entity, `TokenRepository` interface
- [ ] `app/domain/events/user_created.go` – `UserCreated` event


## Fáze 5: Bus

- [ ] `app/bus/bus.go` – `Bus` struct, `Middleware` type, `New()`
- [ ] `app/bus/exec.go` – `Exec[R]()` generická funkce
- [ ] `app/bus/void.go` – `ExecVoid()`
- [ ] `app/bus/middleware_recovery.go` – panic recovery + Sentry report
- [ ] `app/bus/middleware_logging.go` – slog s trace ID, název, trvání
- [ ] `app/bus/middleware_authorize.go` – `Permissioned` / `SkipPermission` / default → error
- [ ] `app/bus/middleware_transaction.go` – BEGIN/COMMIT/ROLLBACK přes context
- [ ] `app/bus/middleware_events.go` – flush `EventCollector` po commitu, async dispatch


## Fáze 6: Security (adaptery)

- [ ] `app/security/password_service.go` – bcrypt, implementuje `domain.PasswordHasher`
- [ ] `app/security/jwt_service.go` – `GenerateAccessToken()`, `ValidateAccessToken()`, `GenerateRefreshToken()` (přes `crypto/rand.Text()`)
- [ ] `app/security/permission_checker.go` – implementuje `domain.PermissionChecker`, čte `AuthClaims` z contextu


## Fáze 7: SQLite repozitáře (adaptery)

- [ ] `app/sqlite/user_repository.go` – `SqliteUserRepository`, implementuje `domain.UserRepository` (Save, Update, Delete, FindByID, FindByNickname, FindAllActive, FindAll)
- [ ] `app/sqlite/token_repository.go` – `SqliteTokenRepository`, implementuje `domain.TokenRepository` (Save, FindByHash, DeleteByUserID, DeleteExpired)
- [ ] Transakce přes `database.TxFromContext(ctx)`


## Fáze 8: Auth (CQRS + HTTP)

- [ ] `app/command/login.go` – `LoginCommand` (`SkipPermission`), `LoginHandler`, vrací `LoginResult`
- [ ] `app/command/refresh_token.go` – `RefreshTokenCommand` (`SkipPermission`), `RefreshTokenHandler`, rotace tokenu
- [ ] `app/command/logout.go` – `LogoutCommand` (`Permissioned`), `LogoutHandler`, smaže refresh tokeny
- [ ] `app/command/change_password.go` – `ChangePasswordCommand` (`Permissioned`), `ChangePasswordHandler`
- [ ] `app/query/get_profile.go` – `GetProfileQuery` (`Permissioned`), `GetProfileHandler`
- [ ] `app/middleware/auth.go` – JWT Bearer middleware, extrahuje claims do contextu
- [ ] `app/handler/auth_login.go` – POST `/api/v1/auth/login`, set refresh cookie
- [ ] `app/handler/auth_refresh.go` – POST `/api/v1/auth/refresh`, čte cookie
- [ ] `app/handler/auth_logout.go` – POST `/api/v1/auth/logout`
- [ ] `app/handler/profile.go` – GET `/api/v1/profile`, PUT `/api/v1/profile/password`
- [ ] Registrace auth rout v `server.go`


## Fáze 9: User management (CQRS + HTTP)

- [ ] `app/command/create_user.go` – `CreateUserCommand` (`Permissioned: admin.users.create`), `CreateUserHandler` s validací + `UserCreated` event
- [ ] `app/command/update_user.go` – `UpdateUserCommand` (`Permissioned: admin.users.update`)
- [ ] `app/command/delete_user.go` – `DeleteUserCommand` (`Permissioned: admin.users.delete`)
- [ ] `app/query/list_users.go` – `ListUsersQuery` (`Permissioned: admin.users.read`)
- [ ] `app/handler/admin_users.go` – `AdminUsersHandler` (HandleCreate, HandleList, HandleUpdate, HandleDelete)
- [ ] `app/event/send_welcome_email.go` – `SendWelcomeEmailHandler` pro `UserCreated`
- [ ] Registrace admin rout v `server.go` s role guard


## Fáze 10: Wire DI

- [ ] `app/di_container/container_provider.go` – Wire injector (`//go:build wireinject`)
- [ ] Provider: `Config`, `SqliteManager`, `MigrationManager`
- [ ] Provider: `JwtService`, `PasswordService` → `domain.PasswordHasher`, `PermissionChecker` → `domain.PermissionChecker`
- [ ] Provider: `CommandBus` (Recovery → Logging → Authorize → Transaction → DispatchEvents)
- [ ] Provider: `QueryBus` (Recovery → Logging → Authorize)
- [ ] Provider: `EventBus` (Recovery → Logging)
- [ ] Provider: `EventCollector`, event handler registry
- [ ] Provider: `SqliteUserRepository` → `domain.UserRepository`, `SqliteTokenRepository` → `domain.TokenRepository`
- [ ] Provider: všechny command/query handlery
- [ ] Provider: všechny HTTP handlery
- [ ] Provider: HTTP middleware, `Server`, `ServeCommand`, `RootCommand`
- [ ] `make di` – ověřit, že `wire_gen.go` se vygeneruje
- [ ] `./bin/app serve` – ověřit start


## Fáze 11: Frontend (Vue 3 SPA)

- [ ] `package.json` – vue, vue-router, vite, tailwindcss, @tailwindcss/vite, typescript, vue-tsc, eslint, oxlint
- [ ] `index.html` – HTML entry
- [ ] `vite.config.ts` – build do `public/`, proxy `/api` → `localhost:3000`
- [ ] `tsconfig.json`
- [ ] `assets/app.ts` – Vue mount s routerem
- [ ] `assets/tailwind.css`
- [ ] `assets/vue/App.vue` – root komponenta s layoutem
- [ ] `assets/vue/router/index.ts` – routes: login, dashboard, admin, guards
- [ ] `assets/vue/composables/useAuth.ts` – login, logout, refresh, scheduleRefresh, isAuthenticated, currentUser
- [ ] `assets/vue/services/apiFetch.ts` – Authorization header, auto-refresh na 401
- [ ] `assets/vue/views/LoginView.vue` – email + heslo formulář
- [ ] `assets/vue/views/DashboardView.vue` – chráněná stránka
- [ ] `assets/vue/views/ProfileView.vue` – profil + změna hesla
- [ ] `assets/vue/views/AdminUsersView.vue` – CRUD uživatelů
- [ ] `public/embed.go` – `//go:embed *`


## Fáze 12: Integrace + build

- [ ] `Makefile` – všechny targety: install, build, dev, fe-dev, serve, lint, test, arch-check, go-fix, check, di, migrate-*
- [ ] `.go-arch-lint.yml` – dependency pravidla pro všech 15 balíčků
- [ ] `make build` – kompletní pipeline: di → fe-build → go-build
- [ ] `make build-all` – cross-platform (linux/amd64, darwin/arm64, windows/amd64)
- [ ] E2E test: login → API call → refresh → logout
- [ ] `make arch-check` – ověřit dependency pravidla
- [ ] `make check` – kompletní CI pipeline


## Fáze 13: Docker + deployment

- [ ] `docker/release/Dockerfile` – Alpine + binárka + SQLite volume
- [ ] `docker-compose.yml` – service s volume pro DB
- [ ] Build: `docker build -f docker/release/Dockerfile -t app:latest .`
- [ ] Test: `docker compose up -d`, ověřit health check


## Fáze 14: Kvalita kódu

- [ ] `.eslintrc` / `eslint.config.ts` – ESLint + oxlint
- [ ] `.golangci.yml` – golangci-lint konfigurace
- [ ] `go fix ./...` – Go 1.26 modernizace
- [ ] Seed: výchozí admin uživatel (z `.env` nebo hardcoded pro dev)
- [ ] README.md – quick start, make targets
