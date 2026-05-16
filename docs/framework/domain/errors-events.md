---
layout: 'page'
uri: '/framework/domain/errors-events'
position: 3
slug: 'framework-domain-errors-events'
parent: 'framework-domain'
navTitle: 'Errors & Events'
title: 'Errors & Events'
description: 'Doménové error typy (ValidationError, AuthError, PermissionError) a domain events s EventCollector.'
---

# Errors & Events


## Proč

Doménové errory nesou sémantiku chyby (validace, oprávnění) bez závislosti na HTTP. Domain events umožňují asynchronní side-effects (notifikace, audit) po úspěšném commandu -- pokud command selže, eventy se zahodí.


## Jak

### ValidationError

Vstupní validace a business pravidla. Žije v `domain/shared/errors.go`.

```go
package shared

type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string      { return e.Message }
func (e *ValidationError) HTTPStatus() int    { return 400 }
func (e *ValidationError) ErrorField() string { return e.Field }
```

Metoda `ErrorField()` implementuje `response.FieldError` -- response writer díky tomu pošle chybu pod klíčem podle pole (`{"nickname": "..."}`), takže frontend ji namapuje rovnou na příslušný input. Když je `Field` prázdný, chyba spadne do `general` klíče. Detaily v [Forms guide](/guides/forms).

Použití ve value objects:

```go
nickname, err := user.NewNickname("")
// err = &shared.ValidationError{Field: "nickname", Message: "nickname is required"}
```


### AuthError

Nejsi autentizován (chybějící / neplatné / expirované credentials). Mapuje na HTTP 401.

```go
type AuthError struct {
    Message string
}

func (e *AuthError) Error() string   { return e.Message }
func (e *AuthError) HTTPStatus() int { return 401 }
```

Použití: JWT middleware (neplatný Bearer), command handlery (neznámý login, expirovaný refresh token, missing claims).


### PermissionError

Jsi autentizován, ale nemáš právo na danou operaci. Mapuje na HTTP 403.

```go
type PermissionError struct {
    Message string
}

func (e *PermissionError) Error() string   { return e.Message }
func (e *PermissionError) HTTPStatus() int { return 403 }
```

Použití: `PermissionChecker` v bus `AuthorizeMiddleware` (role nemá požadovanou permission). HTTP role guard middleware není potřeba -- bus middleware to vynucuje sám pro každý command/query.


### DomainEvent interface

Žije v `domain/shared/event.go`. Eventy jsou čisté data structs s primitivy (serializovatelné).

```go
package shared

type DomainEvent interface {
    EventName() string
    OccurredAt() time.Time
}
```


### Příklad eventu

Žije v `domain/user/user_created.go`:

```go
package user

type UserCreated struct {
    UserID    string
    Nickname  string
    Email     string
    Role      string
    Timestamp time.Time
}

func (e UserCreated) EventName() string     { return "user.created" }
func (e UserCreated) OccurredAt() time.Time { return e.Timestamp }
```


### EventCollector

Sbírá eventy v rámci jednoho command dispatch. Žije v `domain/shared/event.go`. Per-request instanci vytváří `DispatchEventsMiddleware` a ukládá ji do `ctx`; handlery ji čtou helperem `EventCollectorFromContext(ctx)`.

```go
type EventCollector struct { /* mutex + slice */ }

func NewEventCollector() *EventCollector
func (c *EventCollector) Collect(event DomainEvent)
func (c *EventCollector) Flush() []DomainEvent

func ContextWithEventCollector(ctx context.Context) (context.Context, *EventCollector)
func EventCollectorFromContext(ctx context.Context) *EventCollector
```

Použití v command handleru:

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... vytvoření uživatele ...
    shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{
        UserID:    u.ID,
        Nickname:  u.Nickname,
        Email:     u.Email,
        Role:      u.Role,
        Timestamp: time.Now(),
    })
    return nil
}
```

Pokud handler běží mimo bus (např. CLI `create-user`, který bus bypassuje), `EventCollectorFromContext` vrátí throwaway collector — `Collect` projde, ale eventy nikam nejdou.


### Životní cyklus eventů

```
1. DispatchEventsMiddleware vytvoří per-request EventCollector v ctx
2. TransactionMiddleware otevře transakci
3. Command handler volá EventCollectorFromContext(ctx).Collect(event)
4. TransactionMiddleware commitne (nebo rollbackne při chybě)
5. DispatchEventsMiddleware: pokud commit OK, flushne eventy -> EventBus.Dispatch (synchronně)
6. Event handler zpracuje side-effect (email, notifikace)

Pokud command nebo commit selže, chyba propaguje zpět skrz DispatchEvents -> flush se přeskočí, eventy se zahodí.
```


## Detaily

### Error -> HTTP mapování

Doménové errory implementují `HTTPStatus() int` metodu implicitně (Go duck typing). Presentation vrstva (`response/` balíček) definuje vlastní `HTTPError` interface a mapuje errory na HTTP status kódy -- domain nepotřebuje importovat response.

| Error | Status | Kdy |
|---|---|---|
| `*shared.ValidationError` | 400 | Value object validace, business pravidla |
| `*shared.AuthError` | 401 | Chybí/neplatné/expirované credentials, missing claims |
| `*shared.PermissionError` | 403 | Autentizován, ale nemá právo (role neodpovídá) |
| Ostatní | 500 | Systémové chyby |

### Event konvence

- Eventy používají jen primitivy (`string`, `time.Time`), ne celé entity ani value objects.
- Jmenování: `<Entity><Action>` (např. `UserCreated`, `TokenRevoked`).
- `EventName()` vrací `<kontext>.<akce>` (např. `"user.created"`).
- Event handler nesmí modifikovat stav v rámci původní transakce -- běží asynchronně po commitu.
- Pro cross-domain komunikaci: kontext B reaguje na event z kontextu A bez importu A (používá jen primitivní data z eventu).
