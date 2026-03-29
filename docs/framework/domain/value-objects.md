---
layout: 'page'
uri: '/framework/domain/value-objects'
position: 2
slug: 'framework-domain-value-objects'
parent: 'framework-domain'
navTitle: 'Value Objects'
title: 'Value Objects'
description: 'Value objects se vstupní validací – Nickname, Role.'
---

# Value Objects

Nelze je vytvořit v nevalidním stavu – validace probíhá v konstruktoru. Vrací `ValidationError` při nevalidním vstupu.


## Nickname

```go
// domain/nickname.go

type Nickname string

func NewNickname(s string) (Nickname, error) {
    if s == "" {
        return "", &ValidationError{Field: "nickname", Message: "přezdívka je povinná"}
    }
    if len(s) > 50 {
        return "", &ValidationError{Field: "nickname", Message: "max 50 znaků"}
    }
    return Nickname(s), nil
}
```


## Role

```go
// domain/role.go

type Role string

const (
    RoleAdmin Role = "admin"
    RoleUser  Role = "user"
)

func NewRole(s string) (Role, error) {
    switch Role(s) {
    case RoleAdmin, RoleUser:
        return Role(s), nil
    default:
        return "", &ValidationError{Field: "role", Message: "neplatná role"}
    }
}
```


## Kde žije validace

| Typ | Kde | Příklad |
|---|---|---|
| Formát, povinná pole | Value objects | `NewNickname("")` → error |
| Business pravidla s I/O | Command handler | Unique nickname (repo) |
| Oprávnění | Bus AuthorizeMiddleware | `Permissioned` interface |
| Záchranná síť | SQL constraints | `UNIQUE`, `CHECK` |
