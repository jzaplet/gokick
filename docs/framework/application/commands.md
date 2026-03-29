---
layout: 'page'
uri: '/framework/application/commands'
position: 1
slug: 'framework-application-commands'
parent: 'framework-application'
navTitle: 'Commands'
title: 'Commands'
description: 'Balíček command/ – write operace, validace, Permissioned interface.'
---

# Commands

Balíček `command/`. Write operace – mění stav systému. Závisí jen na `domain/`.


## Struktura

Každý command = dva typy v jednom souboru:

- `XxxCommand` – čistý data struct (raw hodnoty z HTTP)
- `XxxHandler` – logika (validace, business pravidla, zápis)


## Příklad

```go
// command/create_user.go

type CreateUserCommand struct {
    Nickname string
    Password string
    Email    *string  // new("user@example.com") – Go 1.26
    Role     string
}

func (c CreateUserCommand) RequiredPermission() string { return "admin.users.create" }

type CreateUserHandler struct {
    repo     domain.UserRepository
    password domain.PasswordHasher
}

func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // 1. Vstupní validace přes domain value objects
    nickname, err := domain.NewNickname(cmd.Nickname)
    if err != nil {
        return err
    }
    role, err := domain.NewRole(cmd.Role)
    if err != nil {
        return err
    }

    // 2. Business pravidlo (I/O)
    existing, _ := h.repo.FindByNickname(ctx, string(nickname))
    if existing != nil {
        return &domain.ValidationError{Field: "nickname", Message: "přezdívka již existuje"}
    }

    // 3. Vytvoření entity + zápis
    hash, err := h.password.Hash(cmd.Password)
    if err != nil {
        return err
    }
    user := domain.NewUser(nickname, hash, cmd.Email, role)
    return h.repo.Save(ctx, user)
}
```


## Pravidla

- Command struct nemá logiku – jen data
- Handler závisí jen na `domain/` interfaces
- Handler neimportuje `security/`, `sqlite/` ani `bus/`
- Permission deklaruje přes `Permissioned` interface – kontrola v bus middleware
- Validace přes domain value objects (formát) + repo queries (business)
