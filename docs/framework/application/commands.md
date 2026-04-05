---
layout: 'page'
uri: '/framework/application/commands'
position: 1
slug: 'framework-application-commands'
parent: 'framework-application'
navTitle: 'Commands'
title: 'Commands'
description: 'Balíček application/command/ – write operace, validace, Permissioned interface.'
---

# Commands

Balíček `application/command/`. Write operace – mění stav systému. Závisí jen na `domain/`.


## Struktura

Každý command = dva typy v jednom souboru:

- `XxxCommand` – čistý data struct (raw hodnoty z HTTP)
- `XxxHandler` – logika (validace, business pravidla, zápis)


## Příklad

```go
// application/command/command_create_user.go

type CreateUserCommand struct {
    Nickname string
    Password string
    Email    string
    Role     string
}

func (c CreateUserCommand) RequiredPermission() string { return "admin.users.create" }

type CreateUserHandler struct {
    repo     user.Repository
    password shared.PasswordHasher
}

func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // 1. Vstupní validace přes domain value objects
    nickname, err := user.NewNickname(cmd.Nickname)
    if err != nil {
        return err
    }
    role, err := user.NewRole(cmd.Role)
    if err != nil {
        return err
    }

    // 2. Business pravidlo (I/O)
    existing, _ := h.repo.FindByNickname(ctx, string(nickname))
    if existing != nil {
        return &shared.ValidationError{Field: "nickname", Message: "přezdívka již existuje"}
    }

    // 3. Vytvoření entity + zápis
    hash, err := h.password.Hash(cmd.Password)
    if err != nil {
        return err
    }
    u := user.NewUser(nickname, hash, cmd.Email, role)
    return h.repo.Save(ctx, u)
}
```


## Pravidla

- Command struct nemá logiku – jen data
- Handler závisí jen na `domain/` interfaces
- Handler neimportuje `infrastructure/security/`, `infrastructure/sqlite/` ani `application/bus/`
- Permission deklaruje přes `Permissioned` interface – kontrola v bus middleware
- Validace přes domain value objects (formát) + repo queries (business)
