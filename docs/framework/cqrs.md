---
layout: 'page'
uri: '/framework/cqrs'
position: 5
slug: 'framework-cqrs'
parent: 'framework'
navTitle: 'CQRS + Bus'
title: 'CQRS + Bus'
description: 'CQRS pattern s bus middleware chain – commands, queries, logging, transakce.'
---

# CQRS + Bus

Oddělení čtení a zápisu na úrovni aplikační logiky. Komunikace mezi HTTP handlery a CQRS handlery probíhá přes bus s middleware chain.


## Command (write)

Mění stav systému. Struktura: `XxxCommand` (čistý data struct) + `XxxHandler` (logika).

Command handler orchestruje: validace přes domain value objects → business preconditions (I/O) → zápis.

```go
// command/create_user.go

// Command je čistý data struct – raw hodnoty z HTTP requestu.
type CreateUserCommand struct {
    Nickname string
    Password string
    Email    *string  // new("user@example.com") – Go 1.26 syntax
    Role     string
}

type CreateUserHandler struct {
    repo     domain.UserRepository
    password *security.PasswordService
}

func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // 1. Vstupní validace přes domain value objects
    nickname, err := domain.NewNickname(cmd.Nickname)
    if err != nil {
        return err // → ValidationError → 400
    }
    role, err := domain.NewRole(cmd.Role)
    if err != nil {
        return err
    }

    // 2. Business pravidlo (potřebuje I/O)
    existing, _ := h.repo.FindByNickname(ctx, string(nickname))
    if existing != nil {
        return &domain.ValidationError{Field: "nickname", Message: "přezdívka již existuje"}
    }

    // 3. Vytvoření entity (nemůže selhat – value objects jsou validní)
    hash, err := h.password.Hash(cmd.Password)
    if err != nil {
        return err
    }
    user := domain.NewUser(nickname, hash, cmd.Email, role)
    return h.repo.Save(ctx, user)
}
```


## Query (read)

Čte stav systému, nemění ho. Struktura: `XxxQuery` (filtry) + `XxxHandler` (logika).

```go
// query/list_users.go
type ListUsersQuery struct{}

type ListUsersHandler struct {
    repo domain.UserRepository
}

func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsersQuery) ([]domain.User, error) {
    return h.repo.FindAll(ctx)
}
```


## Bus

Bus přidává middleware chain kolem command/query handlerů. Go nemá generické metody na struct – řešení: top-level generické funkce.

```go
// bus/bus.go
type Middleware func(ctx context.Context, name string, next func(ctx context.Context) (any, error)) (any, error)

type Bus struct {
    middlewares []Middleware
}

func New(middlewares ...Middleware) *Bus
```

```go
// bus/command.go – typově bezpečný dispatch
func Exec[R any](ctx context.Context, b *Bus, name string, fn func(ctx context.Context) (R, error)) (R, error)

// bus/void.go – zkratka pro commandy bez návratové hodnoty
func ExecVoid(ctx context.Context, b *Bus, name string, fn func(ctx context.Context) error) error
```


### Bus middleware

```go
// bus/middleware_logging.go
// Pro fan-out logging lze použít slog.NewMultiHandler() (Go 1.26) – zápis do více cílů současně.
func LoggingMiddleware(logger *slog.Logger) Middleware

// bus/middleware_recovery.go
func RecoveryMiddleware(logger *slog.Logger) Middleware

// bus/middleware_transaction.go
func TransactionMiddleware(db *database.SqliteManager) Middleware
```


### Dva bus instance

Wire DI vytvoří dva bus instance s různými middleware:

- **CommandBus** = Recovery → Logging → Transaction
- **QueryBus** = Recovery → Logging (bez transakce – jen čte)

```go
// di_container/container_provider.go
func provideCommandBus(logger *slog.Logger, db *database.SqliteManager) *bus.Bus {
    return bus.New(
        bus.RecoveryMiddleware(logger),
        bus.LoggingMiddleware(logger),
        bus.TransactionMiddleware(db),
    )
}

func provideQueryBus(logger *slog.Logger) *bus.Bus {
    return bus.New(
        bus.RecoveryMiddleware(logger),
        bus.LoggingMiddleware(logger),
    )
}
```


## Volání z HTTP handlerů

HTTP handler deserializuje vstup, zavolá přes bus a při chybě deleguje na `response.HandleError` – mapování error→status je centralizované:

```go
// handler/admin_users.go
func (h *AdminUsersHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
    var cmd command.CreateUserCommand
    if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
        response.HandleError(w, err)
        return
    }

    err := bus.ExecVoid(r.Context(), h.commandBus, "CreateUser", func(ctx context.Context) error {
        return h.createUser.Handle(ctx, cmd)
    })
    if err != nil {
        response.HandleError(w, err)
        return
    }
    response.JSON(w, http.StatusCreated, nil)
}

func (h *AdminUsersHandler) HandleList(w http.ResponseWriter, r *http.Request) {
    users, err := bus.Exec[[]domain.User](r.Context(), h.queryBus, "ListUsers", func(ctx context.Context) ([]domain.User, error) {
        return h.listUsers.Handle(ctx, query.ListUsersQuery{})
    })
    if err != nil {
        response.HandleError(w, err)
        return
    }
    response.JSON(w, http.StatusOK, users)
}
```


## Tok requestu

```
HTTP Request
  → HTTP middleware (CORS, logging, JWT auth)
    → HTTP handler (handler/)
      → bus.Exec / bus.ExecVoid
        → Bus middleware: Recovery → Logging → Transaction
          → Command/Query Handler
            → Repository interface (domain/)
              → SQLite implementace (sqlite/)
```


## Pragmatické výjimky

`LoginCommand` vrací `*LoginResult` (token + user info). Striktní CQRS říká "command nevrací data", ale `bus.Exec[*command.LoginResult]` to elegantně řeší bez zbytečné složitosti.
