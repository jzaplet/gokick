---
layout: 'page'
uri: '/framework/domain'
position: 4
slug: 'framework-domain'
parent: 'framework'
navTitle: 'Domain'
title: 'Domain'
description: 'Doménová vrstva – entity, value objects, repository interfaces, validace.'
---

# Domain

Jádro aplikace. Balíček `domain/` obsahuje entity, value objects, repository interfaces a validační error typ. **Nemá žádné závislosti** na ostatních balíčcích – závisí jen na stdlib a `uuid`.


## Value objects

Value objects zajišťují, že doménové hodnoty jsou vždy validní. Nelze je vytvořit v nevalidním stavu – validace probíhá v konstruktoru.

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


## ValidationError

Doménový error typ. Implementuje `HTTPError` interface z `response/` balíčku (implicitně, Go duck typing – žádný import):

```go
// domain/errors.go

type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string   { return e.Message }
func (e *ValidationError) HTTPStatus() int { return 400 }
```

HTTP handler pak volá jen `response.HandleError(w, err)` – mapování error→status je centralizované v `response/` balíčku. Viz [Adaptery – Response helpery](/framework/adapters).


## Entity

Entity přijímají value objects v konstruktoru – nelze vytvořit entitu s nevalidními daty.

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

// Konstruktor přijímá value objects – formátová validace už proběhla.
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


## Repository interfaces

Interfaces definují _co_ systém umí, ne _jak_. Implementace žijí v `sqlite/` balíčku.

```go
// domain/user.go

type UserRepository interface {
    Save(ctx context.Context, user *User) error
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*User, error)
    FindByNickname(ctx context.Context, nickname string) (*User, error)
    FindAllActive(ctx context.Context) ([]User, error)
    FindAll(ctx context.Context) ([]User, error)
}
```

```go
// domain/auth.go

type TokenRepository interface {
    Save(ctx context.Context, token *RefreshToken) error
    FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
    DeleteByUserID(ctx context.Context, userID string) error
    DeleteExpired(ctx context.Context) error
}
```


## Kde žije validace

Validace je distribuovaná do správných míst, žádná extra vrstva:

| Typ validace | Kde | Příklad |
|---|---|---|
| Formát, povinná pole | Domain value objects | `NewNickname("")` → error |
| Business pravidla s I/O | Command handler | Unique nickname check (potřebuje repo) |
| Poslední záchranná síť | SQL constraints | `UNIQUE`, `CHECK`, `NOT NULL` |
