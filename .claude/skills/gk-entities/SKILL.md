---
layout: 'page'
uri: '/skills/gk-entities'
position: 10
slug: 'skills-gk-entities'
parent: 'skills-domain'
navTitle: 'gk-entities'
title: 'GK — Entities & Value Objects'
description: 'Doménové modelování — entity (db tagy pro sqlx) a value objects, které brání vzniku nevalidního objektu. Use when přidáváš nový doménový typ (User, Token, Run…), value object s validací nebo factory funkci, nebo řešíš „kam patří validace".'
name: 'gk-entities'
---

# GK — Entities & Value Objects

Jak se v gokicku modeluje doména: **entity** (objekty s identitou, mapované na DB)
a **value objects** (typy, které se nedají vyrobit v nevalidním stavu).

## What & when
- Sáhni sem, když přidáváš nový doménový typ (entitu jako `User`/`Run`, nebo
  value object jako `Nickname`), píšeš factory funkci (`NewUser`), nebo řešíš
  „kde má žít validace formátu vs. business pravidlo".
- NEtýká se: repozitářů (to je infrastruktura — `/gk-database`), command/query
  handlerů a permissions (`/gk-commands`, `/gk-queries`), ani doménových
  událostí na bus (`/gk-events`).

## For non-tech / juniors
**Entita** je doménový objekt, který má identitu (ID) a něco v životě reprezentuje
— uživatel, refresh token, úloha na pozadí. Žije v DB jako řádek tabulky.

**Value object** je malý typ, který reprezentuje hodnotu a sám si hlídá, že je
platná. Místo holého `string` pro přezdívku máš typ `Nickname`, který vyrobíš
jen přes `NewNickname(...)`. Když je vstup špatný (prázdná přezdívka, moc dlouhá),
konstruktor vrátí chybu a žádný objekt nevznikne. Výhoda: jakmile někde držíš
`Nickname`, máš jistotu, že je validní — nemusí se to znovu kontrolovat. „Nelze
postavit objekt v neplatném stavu."

Analogie: value object je formulářové políčko, které tě nepustí dál, dokud
nevyplníš správnou hodnotu. Entita je celý vyplněný formulář s razítkem (ID).

## How it works
**Bounded contexts** — každá entita má vlastní balíček pod `app/domain/`:
`domain/user/`, `domain/token/`, `domain/run/`. Mezi konteyty se **nesmí
importovat** (`user/` nesmí znát `token/`); sdílené typy žijí v `domain/shared/`.

**Entity** (`app/domain/user/user.go`, `domain/token/refresh_token.go`,
`domain/run/run.go`):
- Struct má `db:"..."` tagy — `sqlx` podle nich automaticky scanuje řádky DB do
  struktury. Příklad: `Nickname string \`db:"nickname"\``.
- ID je `string` (UUID). `User`/`RefreshToken` používají `uuid.New().String()`,
  `Run` `uuid.NewString()` (UUIDv7).
- Entita nemá metody se side-effecty (žádné `Save`/`Load`) — to dělá repository.
- Nullable sloupce: `Run.CompletedAt *time.Time` (nil = nehotovo),
  `RefreshToken.UsedAt *time.Time` (marker theft detection). U `User` jsou
  brute-force pole `sql.NullTime` schválně — SQLite je píše jako TEXT a stdlib
  scanner `sql.NullTime` zvládne i string-z-DB i NULL.

**Factory funkce** (`NewUser`, `NewRun`):
- Přijímají **value objects, ne raw stringy** — `NewUser(nickname Nickname,
  passwordHash string, email Email, role Role)`. Když se caller dostal až k
  factory, data jsou validní.
- `passwordHash` je odvozený stav (produkt `PasswordHasher`), ne value object —
  raw heslo se validuje přes `Password` VO těsně před hashováním.

**Value objects** (`domain/user/nickname.go`, `role.go`, `email.go`, `password.go`):
- Typ je `type Nickname string` + konstruktor `func NewNickname(s string)
  (Nickname, error)`.
- Při nevalidním vstupu vrací `*shared.ValidationError{Field, Message}` —
  `Field` se na FE mapuje na konkrétní políčko (viz `/gk-forms`).
- Konkrétně:
  - `Nickname`: povinný, max 50 znaků.
  - `Role`: enum `RoleAdmin`/`RoleUser`, jiná hodnota → chyba.
  - `Email`: **nepovinný** — prázdný řetězec projde; jinak max 254 znaků a musí
    obsahovat `@`. Striktnější (regex/MX) schválně ne.
  - `Password`: validuje **raw** heslo před hashem — povinné, 8–128 znaků.

## Recipe

### Recipe: přidat value object
1. Nový soubor v `app/domain/<context>/<name>.go`, `type X string` (nebo jiný
   primitiv).
2. `func NewX(s string) (X, error)` — validuj formát/délku/povinnost; při chybě
   vrať `&shared.ValidationError{Field: "x", Message: "…"}`.
3. Použij ho v factory a v command handleru (`NewX` se volá tam, kde přijde raw
   vstup od uživatele).

### Recipe: přidat entitu (nový bounded context)
1. Nový balíček `app/domain/<context>/<entity>.go` — struct s `db:"..."` tagy,
   ID jako `string`.
2. Factory `New<Entity>(...)` přijímající value objects, ne raw stringy.
3. Repository **interface** ve stejném balíčku (`repository.go`) — viz
   `/gk-database` pro implementaci a `.go-arch-lint.yml` (nový context = nová
   `domain_<context>` komponenta + `mayDependOn` granty).

## Invariants & pitfalls
- **Validace formátu/povinnosti žije ve value objektu**, ne v handleru. Business
  pravidla s I/O (unique nickname — repo lookup) žijí v command handleru. SQL
  constraints (`UNIQUE`, `CHECK`) jsou jen záchranná síť.
- **Factory bere value objects, ne raw stringy** — jinak může vzniknout entita z
  nevaliddních dat.
- **Žádné cross-context importy** — `domain/user/` nesmí importovat `domain/token/`.
  Sdílené typy → `domain/shared/`.
- **Value object vrací `*shared.ValidationError`** (ne `errors.New`) — jen tak se
  na FE chyba namapuje na správné políčko (`response.HandleError` → 400).
- **Entita nemá I/O metody** — perzistence patří do repository.
- **`LoginCommand` NEvaliduje heslo přes `Password` VO** — jen porovnává se
  stored hashem. Validace pravidel při loginu by zamkla existující účty po změně
  pravidel. `Password` se používá v `CreateUserCommand` a `ChangePasswordCommand`.
- **Pozor na not-found konvenci repozitáře:** `FindByNickname` vrací `nil, nil`
  (nenalezeno není chyba), ale `user.Repository.FindByID` vrací `*shared.ValidationError`
  (`user not found`) — pozor, není to univerzální pravidlo: `run.Repository.FindByID`
  naopak vrací `nil, nil`. Detail je v implementaci, ale ovlivňuje, jak entitu
  konzumuješ.

## Related
- Sousední skills: `/gk-database` (repository implementace, `r.Conn(ctx)`),
  `/gk-commands` + `/gk-queries` (handlery, permissions, kde žijí business
  pravidla), `/gk-forms` (mapování `ValidationError.Field` na FE políčka),
  `/gk-events` (doménové události jako `UserCreated`).
- Kód: `app/domain/user/` (`user.go`, `nickname.go`, `role.go`, `email.go`,
  `password.go`, `user_created.go`, `repository.go`), `app/domain/token/`
  (`refresh_token.go`, `repository.go`), `app/domain/run/` (`run.go`,
  `repository.go`), `app/domain/shared/` (`ValidationError`)
