---
layout: 'page'
uri: '/framework/domain/models'
position: 1
slug: 'framework-domain-models'
parent: 'framework-domain'
navTitle: 'Entity'
title: 'Entity'
description: 'Doménové entity – User, RefreshToken.'
---

# Entity

Entity přijímají value objects v konstruktoru – nelze vytvořit entitu s nevalidními daty.


## User

```go
// domain/user.go

type User struct {
    ID           string
    Nickname     string
    PasswordHash string
    Email        *string    `json:"email,omitzero"`  // omitzero (Go 1.24)
    Role         string
    Active       bool
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

func NewUser(nickname Nickname, passwordHash string, email *string, role Role) *User {
    return &User{
        ID:           uuid.New().String(),
        Nickname:     string(nickname),
        PasswordHash: passwordHash,
        Email:        email,
        Role:         string(role),
        Active:       true,
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
    }
}
```


## RefreshToken

```go
// domain/auth.go

type RefreshToken struct {
    ID        string
    UserID    string
    TokenHash string
    ExpiresAt time.Time
    CreatedAt time.Time
}
```
