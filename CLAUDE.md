# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Documentation Reference (`docs/framework/`)

- **Overview:** [Architecture](docs/framework/overview/architecture.md), [Layers](docs/framework/overview/layers.md), [Dev Stack](docs/framework/overview/dev-stack.md)
- **Domain:** [Entity & Value Objects](docs/framework/domain/entities.md), [Interfaces](docs/framework/domain/interfaces.md), [Errors & Events](docs/framework/domain/errors-events.md)
- **Application:** [Bus](docs/framework/application/bus.md), [Commands](docs/framework/application/commands.md), [Queries](docs/framework/application/queries.md), [Event Handlers](docs/framework/application/events.md)
- **Infrastructure:** [Wire DI](docs/framework/infrastructure/wire.md), [Database](docs/framework/infrastructure/database.md), [Security](docs/framework/infrastructure/security.md), [Config](docs/framework/infrastructure/config.md)
- **Presentation:** [Handlers & Middleware](docs/framework/presentation/http-handlers.md), [HTTP Server](docs/framework/presentation/http-server.md), [Console](docs/framework/presentation/console.md)

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
make test                                        # vitest + go test (app/ + cmd/ only)
make lint                                        # ESLint + vue-tsc + golangci-lint + go-arch-lint
make format                                      # ESLint Stylistic fix + golines
go test ./app/infrastructure/security/ -run TestHash  # Single Go test
```

### CLI Commands

```bash
./bin/app serve    # Start HTTP server
./bin/app seed     # Seed database with default data (admin user)
```

### Environment

Copy `.env.example` to `.env`. Key vars: `APP_HTTP_PORT`, `APP_DB_PATH`, `APP_JWT_SECRET`, `APP_CORS_ORIGIN`, `APP_JWT_ACCESS_EXPIRATION`, `APP_JWT_REFRESH_EXPIRATION`, `APP_COOKIE_SECURE`. Full reference: [Config](docs/framework/infrastructure/config.md).

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
| `domain/shared/` | `AuthClaims`, `ValidationError`, `AuthError`, `PermissionError`, `DomainEvent`, `EventCollector`, `PermissionsRegistry`, interfaces (`PasswordHasher`, `PermissionChecker`, `JwtService`, `Transactor`, `Seeder`) |
| `domain/user/` | `User` entity, `Nickname`/`Role` value objects, `Repository` interface, `UserCreated` event |
| `domain/token/` | `RefreshToken` entity, `TokenRepository` interface |

**Conventions:**
- Entity structs have `db:"..."` tags for sqlx scanning
- Value objects return `*shared.ValidationError` on invalid input
- Factory functions (`NewUser`) accept value objects, not raw strings
- Repository interfaces return `nil, nil` for "not found" lookups (except `FindByID` which returns `ValidationError`)
- New bounded context = new package under `domain/` (arch-lint wildcard `domain/**` covers it automatically)

### Application Layer (`app/application/`)

CQRS with three bus types, each with its own middleware chain:

| Bus | Chain | Use |
|-----|-------|-----|
| `CommandBus` | Recovery → Logging → Authorize → Transaction → DispatchEvents | Write operations |
| `QueryBus` | Recovery → Logging → Authorize | Read operations |
| `EventBus` | Recovery → Logging | Side-effects after commit |

**Command pattern** (`application/command/`):
```go
type CreateUserCommand struct { Nickname, Password, Email, Role string }
func (c CreateUserCommand) RequiredPermission() string { return "admin:users:create" }

type CreateUserHandler struct { repo user.Repository; password shared.PasswordHasher }
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error { ... }
```

**Query pattern** (`application/query/`):
```go
type ListUsersQuery struct{}
func (q ListUsersQuery) RequiredPermission() string { return "admin:users:read" }

type ListUsersHandler struct { repo user.Repository }
func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsersQuery) ([]user.User, error) { ... }
```

**Permission rules:**
- Every command/query MUST implement either `shared.Permissioned` (returns required permission string) or `shared.SkipPermission` (explicit opt-out)
- If neither is implemented, `AuthorizeMiddleware` returns error — protects against forgotten declarations
- Admin role has full access; user role is denied `admin:*` permissions

**Event pattern** (`application/event/`):
- Command handlers collect events via `EventCollector.Collect(event)`
- Events dispatch after successful transaction commit (`DispatchEventsMiddleware`)
- On rollback, events are discarded
- Event handlers register on `EventBus` via `eventBus.Register(eventName, handlerFn)`

**Dispatch from HTTP handler:**
```go
// Command (no return value):
bus.ExecVoid(ctx, h.commandBus.Bus, "CreateUser", cmd, func(ctx context.Context) error {
    return h.createUser.Handle(ctx, cmd)
})
// Query (typed return):
bus.Exec[[]user.User](ctx, h.queryBus.Bus, "ListUsers", q, func(ctx context.Context) ([]user.User, error) {
    return h.listUsers.Handle(ctx, q)
})
```

### Infrastructure Layer (`app/infrastructure/`)

| Package | Purpose |
|---------|---------|
| `config/` | `LoadConfig()` from `.env` via godotenv → `*Config` struct |
| `database/` | `SqliteManager` (connection, WAL, foreign keys), `MigrationManager` (Goose), transaction context (`BeginTx`/`Commit`/`Rollback`) |
| `sqlite/` | `BaseRepository` (embed in repos for transparent tx support via `r.Conn(ctx)`), `Seeder` |
| `sqlite/user/` | `user.Repository` implementation |
| `sqlite/token/` | `token.TokenRepository` implementation |
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
wire.Bind(new(token.TokenRepository), new(*sqlitetoken.Repository))
wire.Bind(new(shared.Seeder), new(*sqlite.Seeder))
```

### Presentation Layer (`app/presentation/`)

| Package | Purpose |
|---------|---------|
| `http/handler/` | HTTP handlers — decode JSON, dispatch via bus, return response |
| `http/middleware/` | Trace ID, CORS, CSRF (stdlib Go 1.25), Logging, JWT Auth, Role Guard |
| `http/response/` | `JSON()`, `Error()`, `HandleError()` — maps domain errors to HTTP status |
| `http/server/` | `http.ServeMux` routing, middleware chain assembly |
| `console/` | Cobra CLI commands (`serve`, `seed`) — depend on domain interfaces, not infra |

**Error → HTTP mapping** (duck typing, no import between response/ and domain/):
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
  - **Protected endpoints** → `authFetch<Data, Errors>('POST', '/api/v1/...', { body })` from `@/app-ui/Auth`.
  - **Public endpoints** → `apiFetch<Data, Errors>` from `@/app-ui/Fetch`.
- **Backend error response shape** (via `response.Error()` + `FieldError` interface):
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
3. **`application/command/`** or **`application/query/`** — handler with `Permissioned` or `SkipPermission`
4. **`presentation/http/handler/`** — HTTP handler dispatching via bus
5. **`presentation/http/server/`** — register route
6. **`infrastructure/di/container_provider.go`** — add Wire providers + `wire.Bind` for interfaces
7. **`make di && make arch-check`** — regenerate DI + verify layer rules

No changes needed in `.go-arch-lint.yml` — wildcards (`domain/**`, `command/**`, `sqlite/**`) cover new packages automatically.

## Key Invariants

- **Domain interfaces only.** Command/query handlers, seeders, and CLI commands depend on domain interfaces (`user.Repository`, `shared.Seeder`), never on concrete infrastructure types (`*sqliteuser.Repository`, `*sqlite.Seeder`).
- **Bus dispatch required.** All commands/queries go through the bus — never call handlers directly from HTTP handlers. The bus provides recovery, logging, authorization, transactions, and event dispatch.
- **`r.Conn(ctx)` in repositories.** Always use `r.Conn(ctx)` (from embedded `BaseRepository`), never `r.DB.DB()` directly. This ensures transparent transaction participation.
- **Permission declaration.** Every command/query must declare permissions. Forgetting both `Permissioned` and `SkipPermission` is a runtime error.
- **Events use primitives.** Domain events carry only primitive types (string IDs, timestamps), never entities or value objects.
- **No cross-context imports.** Bounded contexts (`domain/user/`, `domain/token/`) are isolated. Shared types live in `domain/shared/`.
