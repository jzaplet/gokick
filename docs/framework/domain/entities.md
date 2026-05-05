---
layout: 'page'
uri: '/framework/domain/entities'
position: 1
slug: 'framework-domain-entities'
parent: 'framework-domain'
navTitle: 'Entity & Value Objects'
title: 'Entity & Value Objects'
description: 'Doménové entity (User, RefreshToken) a value objects (Nickname, Role, Email, Password).'
---

# Entity & Value Objects


## Proč

Entity reprezentují doménové objekty s identitou. Value objects zajišťují, že data jsou validní už při vytvoření -- nelze konstruovat objekt v neplatném stavu. Validace formátu a povinných polí žije ve value objects, business pravidla s I/O (např. unique nickname) v command handlerech.


## Jak

### User entity

Žije v `domain/user/user.go`. Struct používá `db:"..."` tagy pro sqlx scanning.

```go
package user

type User struct {
    ID           string    `db:"id"`
    Nickname     string    `db:"nickname"`
    PasswordHash string    `db:"password_hash"`
    Email        string    `db:"email"`
    Role         string    `db:"role"`
    Active       bool      `db:"active"`
    CreatedAt    time.Time `db:"created_at"`
    UpdatedAt    time.Time `db:"updated_at"`
}

func NewUser(nickname Nickname, passwordHash string, email Email, role Role) *User {
    return &User{
        ID:           uuid.New().String(),
        Nickname:     string(nickname),
        PasswordHash: passwordHash,
        Email:        string(email),
        Role:         string(role),
        Active:       true,
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
    }
}
```

Factory `NewUser` přijímá value objects (`Nickname`, `Email`, `Role`) -- pokud se caller dostal až sem, data jsou validní. `passwordHash` je odvozený stav (produkt `PasswordHasher`), ne doménový koncept -- raw heslo se validuje přes `Password` VO těsně před hashováním.


### RefreshToken entity

Žije v `domain/token/refresh_token.go`.

```go
package token

type RefreshToken struct {
    ID        string     `db:"id"`
    UserID    string     `db:"user_id"`
    TokenHash string     `db:"token_hash"`
    ExpiresAt time.Time  `db:"expires_at"`
    CreatedAt time.Time  `db:"created_at"`
    UsedAt    *time.Time `db:"used_at"` // marker pro theft detection (rotace + zneužití)
}
```


### Nickname value object

Žije v `domain/user/nickname.go`.

```go
package user

type Nickname string

func NewNickname(s string) (Nickname, error) {
    if s == "" {
        return "", &shared.ValidationError{Field: "nickname", Message: "nickname is required"}
    }
    if len(s) > 50 {
        return "", &shared.ValidationError{Field: "nickname", Message: "nickname must be at most 50 characters"}
    }
    return Nickname(s), nil
}
```


### Role value object

Žije v `domain/user/role.go`.

```go
package user

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
        return "", &shared.ValidationError{Field: "role", Message: "invalid role"}
    }
}
```


### Email value object

Žije v `domain/user/email.go`. Email je **nepovinný** -- prázdný řetězec projde jako prázdný `Email`. Když je hodnota neprázdná, validuje se maximální délka a přítomnost `@`. Striktnější kontrolu (regex, DNS MX lookup) záměrně nedělá -- uživatel přijde na řadu při prvním odeslání mailu.

```go
package user

type Email string

// NewEmail validates the email. Empty string is allowed (email is optional).
func NewEmail(s string) (Email, error) {
    if s == "" {
        return "", nil
    }
    if len(s) > 254 {
        return "", &shared.ValidationError{
            Field:   "email",
            Message: "email must be at most 254 characters",
        }
    }
    if !strings.Contains(s, "@") {
        return "", &shared.ValidationError{Field: "email", Message: "invalid email format"}
    }
    return Email(s), nil
}
```


### Password value object

Žije v `domain/user/password.go`. Validuje **raw** heslo před hashingem -- na už uložený hash se nevztahuje (login jen porovnává).

```go
package user

type Password string

func NewPassword(s string) (Password, error) {
    if s == "" {
        return "", &shared.ValidationError{Field: "password", Message: "password is required"}
    }
    if len(s) < 8 {
        return "", &shared.ValidationError{
            Field:   "password",
            Message: "password must be at least 8 characters",
        }
    }
    if len(s) > 128 {
        return "", &shared.ValidationError{
            Field:   "password",
            Message: "password must be at most 128 characters",
        }
    }
    return Password(s), nil
}
```

Používá se v `CreateUserCommand` (při registraci) a `ChangePasswordCommand` (při změně hesla). `LoginCommand` ho **nepoužívá** -- ten jen porovnává se stored hashem, nevaliduje pravidla (jinak by změna pravidel zamkla existující účty).


## Detaily

### Kde žije validace

| Typ | Kde | Příklad |
|---|---|---|
| Formát, povinná pole | Value objects (`NewNickname`, `NewRole`, `NewEmail`, `NewPassword`) | `NewNickname("")` -> `ValidationError` |
| Business pravidla s I/O | Command handler | Unique nickname (repo lookup) |
| Oprávnění | Bus `AuthorizeMiddleware` | `Permissioned` interface |
| Záchranná síť | SQL constraints | `UNIQUE`, `CHECK` |

### Konvence

- Každá entita žije ve vlastním subdoménovém balíčku (`user/`, `token/`).
- Entity struct má `db:"..."` tagy -- používané `sqlx` pro automatický scanning.
- Value objects vrací `*shared.ValidationError` při nevalidním vstupu.
- Factory funkce (např. `NewUser`) přijímají value objects, ne raw stringy.
- Entity nemá metody s side-effecty (žádné Save, Load) -- to je zodpovědnost repository.
