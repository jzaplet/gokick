---
layout: 'page'
uri: '/framework/overview/cross-domain'
position: 5
slug: 'framework-overview-cross-domain'
parent: 'framework-overview'
navTitle: 'Cross-domain izolace'
title: 'Cross-domain izolace'
description: 'Izolace bounded contextů – pravidla, komunikace přes bus, go-arch-lint.'
---

# Cross-domain izolace

Pravidla pro oddělení bounded contextů v doméně. Když aplikace přeroste jeden kontext, doménové balíčky se rozdělí do subdomén s přísnými pravidly závislostí.


## Kdy rozdělit

Jeden doménový kontext stačí dokud:
- Všechny entity sdílejí stejnou databázi a transakční hranici
- Neexistují dvě nezávislé části s vlastními business pravidly
- Tým je malý a pracuje na všem současně

Rozdělit je potřeba když:
- Dva celky mají vlastní životní cyklus (např. "users" vs "billing")
- Entita z jednoho celku potřebuje jen ID z druhého, ne celý objekt
- Změna v jednom celku nemá ovlivnit druhý


## Struktura

Při více kontextech se `domain/` rozdělí do subdomén. Sdílené typy (errors, events, auth) zůstávají v kořeni.

```
app/
├── domain/
│   ├── user/                  # Bounded context: users
│   │   ├── user.go            # User entity + UserRepository
│   │   ├── nickname.go        # Nickname value object
│   │   └── role.go            # Role value object
│   │
│   ├── order/                 # Bounded context: orders (priklad)
│   │   ├── order.go           # Order entity + OrderRepository
│   │   └── status.go          # OrderStatus value object
│   │
│   ├── errors.go              # Sdílené: ValidationError, AuthError
│   ├── auth_context.go        # Sdílené: AuthClaims
│   ├── event.go               # Sdílené: DomainEvent, EventCollector
│   ├── password.go            # Sdílené: PasswordHasher
│   └── permission.go          # Sdílené: Permissioned, SkipPermission, PermissionChecker
│
├── command/
│   ├── user/                  # Commands pro user context
│   │   ├── create_user.go
│   │   └── update_user.go
│   └── order/                 # Commands pro order context (priklad)
│       ├── create_order.go
│       └── complete_order.go
│
├── query/
│   ├── user/
│   └── order/
│
└── ...
```


## Klíčové pravidlo

**Bounded context nesmí importovat jiný bounded context.**

`domain/user/` nesmí importovat `domain/order/` a naopak. Stejné pravidlo platí pro `command/user/` vs `command/order/`.


## Komunikace mezi kontexty

Dva povolené mechanismy:


### 1. QueryBus (synchronní)

Command handler v kontextu A potřebuje data z kontextu B → dispatchuje query přes QueryBus:

```go
// command/order/complete_order.go

type CompleteOrderHandler struct {
    orderRepo domain.OrderRepository
    queryBus  *bus.Bus  // pro cross-context query
}

func (h *CompleteOrderHandler) Handle(ctx context.Context, cmd CompleteOrderCommand) error {
    // Potřebuje email uživatele (user context) → query přes bus
    user, err := bus.Exec[*domain.User](ctx, h.queryBus, "GetUserByID",
        query.GetUserByIDQuery{ID: order.UserID},
        func(ctx context.Context) (*domain.User, error) {
            return h.getUserByID.Handle(ctx, query.GetUserByIDQuery{ID: order.UserID})
        },
    )
    // ...
}
```

Handler nikdy přímo nevolá repository z jiného kontextu. Vždy přes bus.


### 2. Domain Events (asynchronní)

Kontext A emituje event, kontext B reaguje přes event handler:

```go
// Event z user kontextu
type UserCreated struct { ... }

// Event handler v order kontextu (reaguje na user event)
type CreateDefaultOrderHandler struct {
    orderRepo domain.OrderRepository
}

func (h *CreateDefaultOrderHandler) Handle(ctx context.Context, event domain.UserCreated) error {
    // Vytvoří výchozí objednávku pro nového uživatele
}
```

Eventy používají jen primitivy (string ID, ne celé entity). Kontext B nepotřebuje importovat kontext A.


## go-arch-lint enforcement

```yaml
# .go-arch-lint.yml – rozšíření pro bounded contexts

components:
  domain_user:
    in: domain/user/**
  domain_order:
    in: domain/order/**
  domain_shared:
    in: domain/*.go
  command_user:
    in: command/user/**
  command_order:
    in: command/order/**

deps:
  domain_user:
    mayDependOn: [domain_shared]
    # NESMÍ: domain_order

  domain_order:
    mayDependOn: [domain_shared]
    # NESMÍ: domain_user

  command_user:
    mayDependOn: [domain_user, domain_shared]
    # NESMÍ: domain_order, command_order

  command_order:
    mayDependOn: [domain_order, domain_shared]
    # NESMÍ: domain_user, command_user
```


## Kontrola

```bash
make arch-check  # go-arch-lint zachytí cross-domain import
```

Pokud `command/order/complete_order.go` importuje `domain/user/`, arch-check selže. Handler musí použít QueryBus.


## Aktuální stav

Skeleton má jeden bounded context – `domain/` je flat bez subdirectories. Pravidla se aplikují **až přibude druhý kontext**. Struktura je připravená na rozšíření bez refactoringu – stačí vytvořit subdirectories a aktualizovat `.go-arch-lint.yml`.
