# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Documentation Reference

Detailed how-to lives in the **`gk-*` skills** (`.claude/skills/`) — invoke `/gk` for the index, or `/gk-<topic>` directly. Each covers one concept (auth, repositories, background work, permissions, observability, frontend, deploy, …) with why/when, a plain-language explanation, how-it-works, a recipe, and pitfalls. Reach for them before writing code in an area you don't know.

Slim human docs (Documan): [Architecture](docs/framework/architecture.md) · [Installation](docs/framework/installation.md) · [Configuration](docs/framework/configuration.md) · flow pages under `docs/framework/` · [Roadmap (GoKick)](docs/framework/gokick-roadmap.md). The skill index is `/gk` (skills live in `.claude/skills/`). New skills follow `.claude/skills/gk/_TEMPLATE.md`.

## Build & Development Commands

```bash
make install          # Download Go deps + install tools (wire, golines, golangci-lint, goose, go-arch-lint)
make dev              # Regenerate Wire DI + build debug binary → bin/app
make build            # Regenerate Wire DI + build release binary (stripped) → bin/app
make serve            # Run bin/app serve (HTTP server on configured port)
make di               # Regenerate Wire DI only (cd app/infrastructure/di && wire)
```

### Database

```bash
make migrate-up                         # Apply pending migrations
make migrate-down                       # Rollback last migration
make migrate-status                     # Show migration status
make migrate-create NAME=create_x_table # Create new migration file
```

Migrations live in `migrations/` (Goose SQL format, embedded into binary). Migrations run automatically on app startup.

### Quality

```bash
make test                                        # vitest + go test (app/ + cmd/ + tools/gk)
make lint                                        # ESLint + vue-tsc + knip + golangci-lint + go-arch-lint + golines format-check + ts-check + boundary-check + errfields-check + docpaths-check + documan-lint
make format                                      # ESLint Stylistic fix + golines + documan-fix
go test ./app/infrastructure/security/ -run TestHash  # Single Go test
```

### CLI Commands

```bash
./bin/app serve              # Start HTTP server + in-process scheduler + durable-task worker
./bin/app worker             # Run only the persistent durable-task worker (no HTTP server)
./bin/app seed               # Seed admin (+ superadmin if APP_SEED_SUPERADMIN_PASSWORD); multitenant → admin gets its own tenant
./bin/app create-user        # Create a user (-n -p [-e] [-r admin|user] [--tenant-id|--tenant-name]); multitenant → tenant required
./bin/app create-superadmin  # Create a platform superadmin (-n -p [-e]) — the only path to one (admin API refuses the role)
./bin/app create-tenant      # Create a tenant and print its id (-n)
```

### Environment

Copy `.env.example` to `.env`. Key vars: `APP_HTTP_PORT`, `APP_DB_PATH`, `APP_JWT_SECRET` (≥ 32 chars), `APP_CORS_ORIGIN`, `APP_JWT_ACCESS_EXPIRATION`, `APP_JWT_REFRESH_EXPIRATION`, `APP_COOKIE_SECURE`, `APP_SEED_ADMIN_PASSWORD` (required by `./bin/app seed`), `APP_SEED_SUPERADMIN_PASSWORD` (optional — seeds a platform superadmin), `APP_MULTITENANCY` (default `false` = single-tenant; `true` = row-level multitenancy, fail-closed enforcement), `APP_SEED_ADMIN_TENANT` (admin's tenant name when multitenant), `APP_TRUST_PROXY_HEADERS` (flip to `true` only behind a trusted reverse proxy — flips IP source for rate limit + audit), `APP_RATE_LIMIT_LOGIN`, `APP_RATE_LIMIT_REFRESH`, `APP_SENTRY_DSN` / `APP_SENTRY_DSN_FRONTEND` (error tracking, empty = off). Full reference: [Configuration](docs/framework/configuration.md); Sentry setup: the `/gk-sentry` skill.

## Architecture

**DDD 4-layer + CQRS** with strict dependency rules enforced by `go-arch-lint` (`.go-arch-lint.yml`). Module path: `gokick`. Go 1.26.0.

### Layers & Dependency Rules

```
presentation --> application --> domain <-- infrastructure
     |                                        ^
     +----------------------------------------+
```

**Domain depends on nothing. Each layer may only import layers above it. `make arch-check` enforces this.**

| Layer | Folder | Imports allowed | Purpose |
|-------|--------|-----------------|---------|
| **Domain** | `app/domain/` | stdlib + uuid only | Entities, value objects, repository interfaces (ports), domain events, shared error types |
| **Application** | `app/application/` | domain | CQRS buses, command/query/event handlers with middleware chain |
| **Infrastructure** | `app/infrastructure/` | domain, config↔database | Adapter implementations: SQLite repos, bcrypt, JWT, config, Wire DI |
| **Presentation** | `app/presentation/` | application, infrastructure, domain | HTTP handlers + middleware, Cobra CLI commands |

### Domain Layer (`app/domain/`)

Bounded contexts in separate packages. **Never import between contexts** (e.g. `user/` must not import `token/`). Cross-context communication goes through bus or domain events.

| Package | Contains |
|---------|----------|
| `domain/shared/` | `AuthClaims` (incl. `TenantID`), `ValidationError`, `AuthError`, `PermissionError`, `DomainEvent`, `EventCollector`, `AuditCollector` + `AuditEvent` / `AuditRecord`, `PermissionsRegistry`, `DefaultTenantID`, interfaces (`PasswordHasher`, `PermissionChecker`, `TokenService`, `TenantResolver`, `Transactor`, `Seeder`, `AuditLogger`, `RunDispatcher`) |
| `domain/user/` | `User` entity, `Nickname`/`Role` (admin/user/**superadmin**) value objects, `Repository` interface, `PlatformRow` read model, `UserCreated` event |
| `domain/token/` | `RefreshToken` entity, `Repository` interface |
| `domain/run/` | `Run` entity, `Repository` interface — the durable background-task primitive (runs OUTSIDE a tx; optional checkpoint, lease + heartbeat, owner-token fencing, cancel, per-attempt timeout) |
| `domain/tenant/` | `Tenant` entity + `Overview` read model, `Repository` interface — row-level multitenancy boundary |

**Conventions:**
- Entity structs have `db:"..."` tags for sqlx scanning
- Value objects return `*shared.ValidationError` on invalid input
- Factory functions (`NewUser`) accept value objects, not raw strings
- Repository interfaces return `nil, nil` for "not found" lookups — uniform across contexts (`user.FindByID` included since F-011)
- New bounded context = new package under `domain/` **plus its own arch-lint component** (`domain_user`, `domain_token`, …). Each context is a separate component precisely so a cross-context import fails the lint — the trade-off is there is no `domain/**` catch-all, so a new context needs a `domain_<ctx>` entry + a `mayDependOn` grant from each consumer (see the checklist below)

### Application Layer (`app/application/`)

CQRS with three bus types, each with its own middleware chain:

| Bus | Chain | Use |
|-----|-------|-----|
| `CommandBus` | Recovery → Logging → Authorize → Tenant → Audit → RunDispatcher → DispatchEvents → Transaction | Write operations |
| `QueryBus` | Recovery → Logging → Authorize → Tenant | Read operations |
| `EventBus` | Recovery → Logging | Side-effects after commit |

**TenantMiddleware** (in `BaseChain`, right after Authorize → covers both command and query bus) resolves the active tenant into ctx so every downstream handler/repo sees it; reads need scoping as much as writes. See the `/gk-multitenancy` skill.

**Audit middleware lives OUTSIDE Transaction** so security-relevant events (login_failed, account_locked, theft_detected) persist even when the business tx rolls back. Audit write failures are logged but never propagated to the caller.

**Command pattern** (`application/<ctx>/command/`):
```go
type CreateUserCommand struct { Nickname, Password, Email, Role string }
func (c CreateUserCommand) RequiredPermission() string { return "admin:users:create" }

type CreateUserHandler struct { repo user.Repository; password shared.PasswordHasher }
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error { ... }
```

**Query pattern** (`application/<ctx>/query/`):
```go
// Grid state as it arrived on the wire; Handle normalizes it into whitelisted
// domain criteria (unknown sort / out-of-range page fall back, never 400).
type ListUsersQuery struct { Page, PerPage int; SortBy, SortDir, Nickname, Email, Role, Active string }
func (q ListUsersQuery) RequiredPermission() string { return "admin:users:read" }

type ListUsersHandler struct { repo user.Repository }
func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsersQuery) (user.ListPage, error) { ... }
```

**Permission rules:**
- Every command/query MUST implement either `shared.Permissioned` (returns required permission string) or `shared.SkipPermission` (explicit opt-out)
- If neither is implemented, `AuthorizeMiddleware` returns error — protects against forgotten declarations
- Role ladder (`shared.IsPermissionAllowedForRole`): `superadmin` has full access incl. `platform:*` (the cross-tenant platform plane); `admin` has `admin:*` and below but **NOT** `platform:*`; `user` is denied both `admin:*` and `platform:*`

**Event pattern** (`application/event/`):
- `DispatchEventsMiddleware` creates a **per-request** `*shared.EventCollector` and stores it in `ctx` (no singleton — race-safe).
- Command handlers read it via `shared.EventCollectorFromContext(ctx).Collect(event)`. Outside any bus (e.g. a handler invoked directly in a test) the helper returns a throwaway collector so handlers never nil-check; inside event/run handlers it returns a forbidden collector that panics on Collect (use `RunDispatcher` there).
- `DispatchEventsMiddleware` wraps `TransactionMiddleware` (outer). After the transaction commits successfully, the middleware flushes and dispatches events via `EventBus` **synchronously**. On rollback or commit failure, events are discarded.
- Event handlers register on `EventBus` via `eventBus.Register(eventName, handlerFn)`. They **must not** call `Collect` themselves — for follow-up async work use `RunDispatcher`.

**Dispatch from HTTP handler:**
```go
// Command (no return value):
bus.DispatchVoid(ctx, h.commandBus, "CreateUser", cmd, func(ctx context.Context) error {
    return h.createUser.Handle(ctx, cmd)
})
// Command (typed return — e.g. a bulk op's affected count):
bus.Dispatch(ctx, h.commandBus, "BulkDeleteUsers", cmd, func(ctx context.Context) (int64, error) {
    return h.bulkDelete.Handle(ctx, cmd)
})
// Query (typed return):
bus.Query(ctx, h.queryBus, "ListUsers", q, func(ctx context.Context) (user.ListPage, error) {
    return h.listUsers.Handle(ctx, q)
})
```

Taking `*CommandBus`/`*QueryBus` makes the bus↔operation pairing compile-checked; the CLI uses `bus.SystemDispatch`/`bus.SystemDispatchVoid` on the `*SystemCommandBus`.

### Infrastructure Layer (`app/infrastructure/`)

| Package | Purpose |
|---------|---------|
| `config/` | `LoadConfig()` from `.env` via godotenv → `*Config` struct |
| `database/` | `SqliteManager` (connection, WAL, `_txlock=immediate`, `busy_timeout`, `foreign_keys` via DSN), `MigrationManager` (Goose), transaction context (`BeginTx`/`Commit`/`Rollback`) |
| `sqlite/` | `BaseRepository` (embed in repos for transparent tx support via `r.Conn(ctx)`) |
| `sqlite/user/` | `user.Repository` impl (incl. `RecordFailedLogin` / `ResetFailedLogin` / `RecordLogin` raw-pool on purpose; tenant-scoped admin reads/writes + cross-tenant platform reads/writes — the `*AcrossTenants` set. The ones that touch EXISTING rows exclude superadmins in the statement itself; `SaveAcrossTenants` (the platform create) has no existing row to exclude, so the superadmin role is refused by `userwrite.Create` instead — the shared body is the floor, and `userwrite.CreateSuperAdmin` is the one sanctioned minter) |
| `sqlite/token/` | `token.Repository` implementation |
| `sqlite/run/` | `run.Repository` implementation (owner-fenced; julianday/ms time discipline shared via `sqlite/sqltime.go`) |
| `sqlite/tenant/` | `tenant.Repository` implementation (row-level multitenancy boundary) |
| `sqlite/audit/` | `shared.AuditLogger` implementation (raw-pool — survives business rollback) |
| `sqlite/seeder/` | `shared.Seeder` impl — admin (+ optional superadmin) seeding; `SeedAdminPassword` / `SeedSuperAdminPassword` / `SeedAdminTenant` / `Multitenant` Wire-distinct types |
| `security/` | `JwtService` (HS256 access + crypto/rand refresh), `PasswordHasher` (SHA-256 prehash + bcrypt), `PermissionChecker` |
| `di/` | Wire compile-time DI. `container_provider.go` (wireinject tag) + generated `wire_gen.go` |

**Repository pattern:**
```go
type Repository struct { sqlite.BaseRepository }  // embed
func (r *Repository) Save(ctx context.Context, u *user.User) error {
    _, err := r.Conn(ctx).NamedExecContext(ctx, query, u)  // tx-aware
    return err
}
```

**Wire binding for interfaces:**
```go
wire.Bind(new(user.Repository), new(*sqliteuser.Repository))
wire.Bind(new(token.Repository), new(*sqlitetoken.Repository))
wire.Bind(new(shared.Seeder), new(*sqliteseeder.Seeder))
```

### Presentation Layer (`app/presentation/`)

| Package | Purpose |
|---------|---------|
| `http/handler/` | HTTP handlers — decode JSON, dispatch via bus, return response |
| `http/middleware/` | Trace ID, IP, ReportScope, Recovery, Security headers, CORS, Logging, JWT Auth, Rate limit — CSRF (stdlib Go 1.25 `http.CrossOriginProtection`) is assembled in `http/server/`; permission checks live in the bus `AuthorizeMiddleware` |
| `http/response/` | injected `*Responder` — `JSON(ctx, w, …)`, `Error(ctx, w, …)`, `HandleError(ctx, w, err)` methods; maps domain errors to HTTP status (encode failures logged, not swallowed) |
| `http/server/` | `http.ServeMux` routing, middleware chain assembly |
| `console/` | Cobra CLI commands (`serve`, `worker`, `seed`, `create-user`, `create-superadmin`, `create-tenant`) — `serve` co-runs the in-process scheduler + worker alongside the HTTP server, sharing one ctx so SIGTERM drains all. The create/seed commands dispatch through the **`SystemCommandBus`** (operator-trusted: no Authorize/Tenant, but transaction + audit + panic→Sentry); with multitenancy on, `create-user` requires a tenant (`--tenant-id` / `--tenant-name`) |

**Error → HTTP mapping** (duck typing — domain errors declare `HTTPStatus()`, response/ matches its local `HTTPError` interface):
- `*shared.ValidationError` → 400
- `*shared.AuthError` → 401 (not authenticated)
- `*shared.PermissionError` → 403 (authenticated but not permitted)
- Other errors → 500

### Frontend (`assets/`)

**Structure:** Domain-based organization:
- `assets/app/<Domain>/Views/` — routed views (orchestrators only — layout + mount components)
- `assets/app/<Domain>/Components/` — domain-specific, self-contained components (forms, cards, widgets)
- `assets/app-ui/` — shared, generic components reusable across domains

**Views are orchestrators.** Keep business logic out of `Views/` — a view mounts a few components, adds layout, maybe reads router/state for prop-passing. Forms live as `<Domain>/Components/XxxForm.vue`, display widgets as `XxxCard.vue`, etc.

**Vue components:**
- Use `<script setup lang="ts">` with proper TypeScript types
- Define form/error types using `type` (not `interface`)
- Use `reactive<Type>()` and `ref<Type>()` with explicit typing
- Props with destructuring: `const { prop1, prop2 = 'default' } = defineProps<Type>()`
- No frontend validation — all validation handled by backend
- Always use `@/` alias for imports — never relative paths
- Always use `for...of` instead of `.forEach()`
- Always use strict comparison (`===`, `!==`)
- Always use explicit boolean checks: `if (x === true)`, never `if (!x)` — use `if (x === false)` or `if (x === null)`
- Never use `as` type casting — use generics instead
- Never use `function` keyword — always arrow functions (`const fn = (): void => { ... }`)
- Never use `class` — use composables, plain objects, and closures instead
- Access index signature properties with bracket notation: `obj['key']` not `obj.key` (enforced by `noPropertyAccessFromIndexSignature`)
- ID is always `string` (UUIDv7), never `number`

**Tailwind class formatting in templates:**
- Long class lists (5+ utilities) use `:class="[...]"` array syntax instead of `class="..."`
- Group related utilities together: layout, sizing, spacing, typography, visual, interactive
- Example:
```html
<div
    :class="[
        'flex items-center justify-center',
        'w-full h-12',
        'px-4 py-2',
        'text-sm font-medium text-white',
        'bg-blue-600 rounded-lg shadow-sm',
        'hover:bg-blue-700 transition-colors cursor-pointer',
    ]"
>
```
- Short classes (1-4 utilities) stay as plain `class="..."`
- Dynamic classes mix with static: `:class="['static classes', dynamicVar]"`

**SVG icons:**
- Break long `d` attribute values across multiple lines (max 120 chars per line)

**Permissions:**
- Never hard-code permission strings (`'admin:users:read'`, etc.) anywhere in `assets/`. Use the `Permission` enum from `@/app/Auth/enums/resources` — it mirrors backend `RequiredPermission()` declarations and is the single source of truth on the frontend.
- `hasPermission` / `hasAllPermissions` / `hasAnyPermission` and `meta.requiresPermission` are typed as `Permission`, so a missing or misspelled value is a compile-time error.
- Adding a new backend permission means adding a matching entry in `resources.ts`.

**Registration in router:**
- Every route must declare `meta.requiresAuth: true|false` — enforced via `AppRoute` type. Mirrors backend `Permissioned`/`SkipPermission` rule.
- Protected routes: `{ requiresAuth: true }`; admin: `{ requiresAuth: true, requiresPermission: 'x:y:z' }`.
- Routes live in `assets/router/routes.ts`, guard in `assets/router/authGuard.ts`.

**Forms, requests & errors:**
- `reactive` for form data, `ref<T>({})` for errors, `ref<boolean>` for `isLoading`.
- Errors type: all fields optional (`?:`). Key absent = no error.
  ```typescript
  type LoginErrors = { general?: string; nickname?: string; password?: string };
  const errors = ref<LoginErrors>({});
  ```
- Clear field error on edit:
  ```typescript
  const clearFieldError = (field: keyof LoginErrors): void => {
      // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
      delete errors.value[field];
  };
  ```
- Requests:
  - **Protected endpoints** → `authFetch<Data, Errors, Body>('POST', '/api/v1/...', { body, validate })` from `@/app-ui/Auth`.
  - **Public endpoints** → `apiFetch<Data, Errors, Body>` from `@/app-ui/Fetch`.
  - **Body requires its generic.** `FetchOptions<TBody = never>` — passing `body` without declaring the third generic does not compile. Declare it with the tsgen-generated request type (`UserFormData`, `LoginRequest`, …); ESLint additionally requires explicit generics on every `authFetch`/`apiFetch` call and forbids inline `body` literals (pass a typed variable). GET/DELETE calls without a body stay two-generic.
  - **Data endpoints require `validate`.** When `Data ≠ null`, options must carry `validate:` — the tsgen-generated guard (`isAdminUser`, `arrayOf(isAdminUser)`, `isLoginResponse`, …) — or the call does not compile; the 2xx body is then checked against the generated contract at runtime. A contract-violating body becomes a `{ general }` failure and reports to Sentry. 204 endpoints (`Data = null`) take no guard.
  - **Failures always merge.** A failure `data` is `TErrors | { general: string }`: the API error body when one arrived, or the synthesized `{ general: … }` for network errors / malformed bodies / contract violations (same `general` key the backend uses for non-field errors). Every `*Errors` type has `general?: string`, so `errors.value = result.data;` stays the one-line merge — no narrowing.
- **Backend error response shape** (via `Responder.Error()` + `FieldError` interface):
  - `ValidationError{Field: "nickname"}` → `{ "nickname": "..." }` — routed to specific field.
  - Any other error → `{ "general": "..." }`.
- **Frontend error handling:** one-line merge.
  ```typescript
  const result = await login<LoginErrors>(form);
  if (result.success === false) {
      errors.value = result.data;   // server keys land in matching form fields
      return;
  }
  ```
- Submit flow: `errors.value = {}` → call → on failure `errors.value = result.data` → toast; on success → toast + redirect.
- Never validate on the frontend — backend is authoritative.

## Development Flow

1. **Feature** — implement (see checklist below), `make di` after DI changes
2. **Architecture** — `make arch-check` to verify layer dependency rules
3. **Code style** — `make lint` + `make format`
4. **Tests** — `make test`

## Adding a New Feature (Checklist)

1. **`domain/<context>/`** — entity, value objects, repository interface
2. **`infrastructure/sqlite/<context>/`** — repository implementation
3. **`application/<ctx>/command/`** or **`application/<ctx>/query/`** — handler with `Permissioned` or `SkipPermission`
4. **`presentation/http/handler/`** — HTTP handler dispatching via bus
5. **`presentation/http/server/`** — register route
6. **`infrastructure/di/container_provider.go`** — add Wire providers + `wire.Bind` for interfaces
7. **`make di && make arch-check`** — regenerate DI + verify layer rules

Broad-glob components auto-cover new sub-packages: a new `application/<ctx>/command/` matches `application/**`, a new handler matches `presentation/http/handler/**`. But the **bounded-context** components are enumerated, not wildcarded — `domain` is split per context (`domain_user`, `domain_token`, `domain_run`, `domain_tenant`) and `sqlite_repos` lists each repo dir — exactly so a cross-context import is caught. So adding a new context (`domain/order/`, `infrastructure/sqlite/order/`) **does** require editing `.go-arch-lint.yml`: add a `domain_order` component, grant it in each consumer's `mayDependOn` (`application`, `sqlite_repos`, `testfx`, …), and add `infrastructure/sqlite/order/**` to `sqlite_repos`. That ~6-line edit is the price of enforcing cross-context isolation in the linter.

## Key Invariants

- **Domain interfaces only.** Command/query handlers, seeders, and CLI commands depend on domain interfaces (`user.Repository`, `shared.Seeder`), never on concrete infrastructure types (`*sqliteuser.Repository`, `*sqliteseeder.Seeder`).
- **Bus dispatch required.** All commands/queries go through a bus — HTTP handlers never call application handlers directly (they use the `CommandBus`/`QueryBus`); the CLI create/seed commands use the `SystemCommandBus` (the CommandBus chain minus Authorize/Tenant). The bus provides recovery, logging, authorization (HTTP), transactions, audit, and event dispatch.
- **`r.Conn(ctx)` in repositories.** Always use `r.Conn(ctx)` (from embedded `BaseRepository`), never `r.DB.DB()` directly. This ensures transparent transaction participation. **Exception:** writes that MUST commit independently of any bus tx — `user.Repository.RecordFailedLogin/ResetFailedLogin/RecordLogin` (login is `SkipTransaction`, so these single-statement writes auto-commit; the raw pool also future-proofs against an in-tx caller) and `audit.Repository.Save` (must survive the business rollback) — use `r.DB.DB()` on purpose. These are the only legitimate raw-pool callers; document the reason in the method comment.
- **Tenant scoping (multitenancy).** Every SQL query on a tenant-owned table (`users`, …) must scope by `tenant_id` (from `r.Tenant(ctx)`) OR carry an inline `/* tenant-scope-exempt: <reason> */` marker — the `zz_tenant_test.go` conformance gate fails CI otherwise (an unclassified table also fails). The resolver supplies the tenant, the repo applies it (no transparent `WHERE` injection); `r.Tenant(ctx)` panics on a missing tenant in multitenant mode (fail-closed). Cross-tenant platform reads carry `tenant-scope-exempt: platform superadmin`. See `/gk-multitenancy`.
- **Permission declaration.** Every command/query must declare permissions. Forgetting both `Permissioned` and `SkipPermission` is a runtime error.
- **Events use primitives.** Domain events carry only primitive types (string IDs, timestamps), never entities or value objects.
- **No cross-context imports.** Bounded contexts (`domain/user/`, `domain/token/`) are isolated. Shared types live in `domain/shared/`.
- **Audit events for security-relevant work.** Handlers that mutate auth state or user records call `shared.AuditCollectorFromContext(ctx).Record(...)`. The collector returns a throwaway when nothing installed one in ctx (e.g. a handler invoked directly in a test — the CLI system bus and the run worker both install real collectors), so the call is always safe. Action names follow the dotted convention `domain.event` (`auth.login.failed`, `user.role_changed`, etc.).
- **Structured logging is statically enforced — there is one logging path.** All logging goes through the injected `*slog.Logger`, built once in `cmd/logger.go`. `.golangci.yml` forbids every alternative at lint time: `fmt.Print*` / `print`/`println` (stdout), the stdlib `log` package, **any** third-party logger (depguard **import allow-list** — a new dependency must be added to it), `slog.New*` outside `cmd/`, the global default logger (incl. `slog.Default()`), `os.Stdout`/`os.Stderr`, and direct file/fd opens (`os.Create`/`os.OpenFile`/`os.WriteFile`/`os.NewFile`/`syscall.Write`) so log data can't leak into a file. Every attribute **key is a Go constant** (sloglint `no-raw-keys`): cross-cutting keys in `shared.LogKey*` (`trace_id`, `user_id`, `command`, `duration_ms`, …); component-specific keys as package-local `logKey*` consts. Keys are `snake_case`, messages are constants, key-value and `slog.Attr` styles are never mixed. Correlation comes from `shared.LogAttrs(ctx)`, durations from `shared.DurationMsAttr`. See the `/gk-logging` skill.
- **Error reporting is for the unexpected only.** Recovered panics (bus + HTTP `RecoveryMiddleware`) and terminal task failures (the run worker, exhausted retries) report to the injected `shared.ErrorReporter` (Sentry, built in `cmd/sentry.go`, a `NopReporter` without `APP_SENTRY_DSN`). Ordinary returned errors — validation, auth, 4xx — must NOT be reported; only the recovery/terminal paths, or the tracker drowns in noise. Same on the frontend (`@sentry/vue`, gated on `VITE_SENTRY_DSN`): Vue errors + unhandled rejections, never handled API 4xx. sentry-go is on the depguard allow-list precisely because it is the one sanctioned non-slog sink.
- **No hard-coded permissions on the frontend.** Every permission reference in `assets/` goes through the `Permission` enum in `assets/app/Auth/enums/resources.ts`. String literals like `'admin:users:read'` in `.vue` / `.ts` files are forbidden.
- **Wire payloads are gkts-annotated named structs.** Every `Responder.JSON` payload and `request.DecodeJSON` target must be a named struct carrying a `//gkts:<path> <TSName>` directive (tsgen generates the TS side; `make ts-gen` after changing a DTO). `make boundary-check` (in `make lint`) fails on inline maps, `any`, or unannotated types — and on going around the boundary: direct `encoding/json` calls, package-level `response.*` functions, method values of the boundary functions (`send := resp.JSON`), and raw handler writes (`w.Write`, `io.WriteString`, `fmt.Fprint*`) inside `presentation/http/**` (matching is type-based, so renaming the variable doesn't help). The only escape is `//gkts:ignore <reason>` standing alone on the line above the call, for endpoints the SPA never consumes (`/health`, `APP_RUN_DEBUG` debug endpoints, the embedded SPA shell); a reasonless, trailing, or unused marker is itself a violation, and markers never silence the `encoding/json`/`response.*` bypasses.
