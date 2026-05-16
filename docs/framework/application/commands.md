---
layout: 'page'
uri: '/framework/application/commands'
position: 2
slug: 'framework-application-commands'
parent: 'framework-application'
navTitle: 'Commands'
title: 'Commands'
description: 'Balíček application/command/ -- write operace, validace, Permissioned interface.'
---

# Commands

## Proč

Commands reprezentují write operace -- mění stav systému. Každý command je čistý data struct + handler s business logikou. Handler závisí výhradně na doménových interfaces (např. `user.Repository`, `shared.PasswordHasher`), nikdy na infrastruktuře.

## Jak

Každý command = dva typy v jednom souboru:

- `XxxCommand` -- data struct (raw hodnoty z HTTP requestu)
- `XxxHandler` -- logika (validace, business pravidla, zápis)

### Příklad

```go
// application/user/command/create_user.go

type CreateUserCommand struct {
    Nickname string
    Password string
    Email    string
    Role     string
}

func (c CreateUserCommand) RequiredPermission() string { return "admin:users:create" }

type CreateUserHandler struct {
    users    user.Repository        // doménový interface
    password shared.PasswordHasher  // doménový interface
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
    password, err := user.NewPassword(cmd.Password)
    if err != nil {
        return err
    }
    email, err := user.NewEmail(cmd.Email)
    if err != nil {
        return err
    }

    // 2. Business pravidlo (I/O) -- unique nickname
    existing, err := h.users.FindByNickname(ctx, string(nickname))
    if err != nil {
        return err
    }
    if existing != nil {
        return &shared.ValidationError{
            Field:   "nickname",
            Message: "user with this nickname already exists",
        }
    }

    // 3. Hash hesla + vytvoření entity + zápis
    hash, err := h.password.Hash(string(password))
    if err != nil {
        return err
    }
    u := user.NewUser(nickname, hash, email, role)
    if err := h.users.Save(ctx, u); err != nil {
        return err
    }

    // 4. Domain event -- per-request collector v ctx, bus ho dispatchne po commitu
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

### Dispatch z HTTP handleru

```go
err := bus.ExecVoid(ctx, h.commandBus.Bus, "CreateUser", cmd, func(ctx context.Context) error {
    return h.createUser.Handle(ctx, cmd)
})
```

## Detaily

- Command struct nemá logiku -- jen data.
- Handler nikdy neimportuje `infrastructure/security/`, `infrastructure/sqlite/` ani `application/bus/`. Wire injektuje doménové interfaces.
- Permission se deklaruje přes `Permissioned` interface -- kontrolu provádí `AuthorizeMiddleware` v busu.
- Validace: domain value objects (formát) + repo queries (unikátnost, existence).
- Transakce a event dispatch řídí bus middleware -- handler o nich neví.
