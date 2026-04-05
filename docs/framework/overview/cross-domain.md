---
layout: 'page'
uri: '/framework/overview/cross-domain'
position: 5
slug: 'framework-overview-cross-domain'
parent: 'framework-overview'
navTitle: 'Cross-domain Isolation'
title: 'Cross-domain Isolation'
description: 'Izolace bounded contextů – pravidla, komunikace přes bus, go-arch-lint.'
---

# Cross-domain Isolation

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

Při více kontextech se `domain/` rozdělí do subdomén. Sdílené typy zůstávají v `domain/shared/`.

```
app/
├── domain/
│   ├── shared/                  # Sdílené: errors, events, auth, interfaces
│   ├── user/                    # Bounded context: users
│   └── order/                   # Bounded context: orders (příklad)
│
├── application/
│   ├── command/
│   │   ├── user/                # Commands pro user context
│   │   └── order/               # Commands pro order context
│   ├── query/
│   │   ├── user/
│   │   └── order/
│   └── ...
```


## Klíčové pravidlo

**Bounded context nesmí importovat jiný bounded context.**

`domain/user/` nesmí importovat `domain/order/` a naopak. Stejné pravidlo platí pro `application/command/user/` vs `application/command/order/`.


## Komunikace mezi kontexty

Dva povolené mechanismy:


### 1. QueryBus (synchronní)

Command handler v kontextu A potřebuje data z kontextu B → dispatchuje query přes QueryBus:

```go
// application/command/order/complete_order.go

type CompleteOrderHandler struct {
    orderRepo order.OrderRepository
    queryBus  *bus.QueryBus
}

func (h *CompleteOrderHandler) Handle(ctx context.Context, cmd CompleteOrderCommand) error {
    user, err := bus.Exec[*user.User](ctx, h.queryBus.Bus, "GetUserByID",
        query.GetUserByIDQuery{ID: order.UserID},
        func(ctx context.Context) (*user.User, error) {
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

// Event handler v order kontextu
type CreateDefaultOrderHandler struct {
    orderRepo order.OrderRepository
}

func (h *CreateDefaultOrderHandler) Handle(ctx context.Context, event user.UserCreated) error {
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
  command_user:
    in: application/command/user/**
  command_order:
    in: application/command/order/**

deps:
  domain_user:
    anyVendorDeps: true
    # NESMÍ: domain_order

  domain_order:
    anyVendorDeps: true
    # NESMÍ: domain_user

  command_user:
    mayDependOn: [domain_user]
    # NESMÍ: domain_order, command_order

  command_order:
    mayDependOn: [domain_order]
    # NESMÍ: domain_user, command_user
```


## Kontrola

```bash
make arch-check  # go-arch-lint zachytí cross-domain import
```

Pokud `application/command/order/complete_order.go` importuje `domain/user/`, arch-check selže. Handler musí použít QueryBus.


## Aktuální stav

Skeleton má dva doménové kontexty – `domain/user/` a `domain/token/` – plus `domain/shared/` pro sdílené typy. Pravidla cross-domain izolace se aplikují **až přibude další kontext**. Struktura je připravená na rozšíření bez refactoringu.
