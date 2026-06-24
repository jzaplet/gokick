---
layout: 'page'
uri: '/skills/gk-feature'
position: 40
slug: 'skills-gk-feature'
parent: 'skills-start'
navTitle: 'gk-feature'
title: 'GK — Feature end-to-end'
description: 'Přidání nové featury end-to-end přes všechny vrstvy bez zapomenuté permission, route nebo DI bindingu. Use when přidáváš nový endpoint / command / query (CRUD nad entitou, nová akce) a potřebuješ checklist napříč domain → repo → command/query → handler → route → DI.'
name: 'gk-feature'
---

# GK — Feature end-to-end

Jak protáhnout novou featuru všemi čtyřmi vrstvami (DDD + CQRS) tak, aby nic
nevypadlo — od entity v doméně až po zaregistrovanou HTTP routu a `make di`.

## What & when

- Sáhni sem, když přidáváš **novou akci nad daty**: nový endpoint, nový
  command (zápis) nebo query (čtení), CRUD nad existující entitou, nebo úplně
  nový bounded context.
- Tohle je **rozcestník a checklist** — proč/jak jednotlivých vrstev řeší
  detailnější skills: `/gk-domain` (entita, value objects), `/gk-bus`
  (command/query/permission), `/gk-handlers` (HTTP vrstva), `/gk-config`
  (Wire DI). Sem chodíš pro pořadí kroků a aby ti nic neuteklo.
- NEtýká se to periodické práce na pozadí (`/gk-scheduler`, `/gk-jobs`) ani
  čistě frontendové změny.

## For non-tech / juniors

Aplikace je rozdělená do čtyř „pater" (vrstev). Data tečou shora dolů: HTTP
request přijde nahoře, propadne přes „business" logiku až dolů k databázi a
výsledek se vrátí zpět. Každá featura proto musí mít kousek v každém patře —
jako když stavíš výtahovou šachtu: musíš ji provrtat **všemi** podlažími, jinak
výtah nedojede. Tenhle skill je seznam podlaží, kterými musíš projít, plus
zvonek na konci (`make di`), který všechno propojí dohromady.

## How it works

Vrstvy a kam co patří (potvrzeno v repu):

| Vrstva | Složka | Co tam přidáš |
|---|---|---|
| **Domain** | `app/domain/<ctx>/` | entita, value objects, `Repository` interface |
| **Infrastructure** | `app/infrastructure/sqlite/<ctx>/` | implementace repozitáře |
| **Application** | `app/application/<ctx>/command/` nebo `…/query/` | handler + `RequiredPermission()` |
| **Presentation** | `app/presentation/http/handler/` + `…/server/` | HTTP handler + route |
| **DI** | `app/infrastructure/di/container_provider.go` | provider + `wire.Bind` + zápis do registrů |

**Pozor — application vrstva je dělená po bounded contextu**, ne ploché
`application/command/`. Reálné cesty: `app/application/user/command/create_user.go`,
`app/application/user/query/list_users.go`. Importy: `usercmd "gokick/app/application/user/command"`.

**Command** (`app/application/user/command/create_user.go`) = zápis. Struct nese
data, metoda `RequiredPermission() string` deklaruje permission, `Handle(ctx, cmd) error`
dělá práci (validace přes value objects, `repo.Save`, sbírání eventů/auditu).

**Query** (`app/application/user/query/list_users.go`) = čtení. `Handle(ctx, q) (R, error)`
vrací data.

**Handler** (`app/presentation/http/handler/admin_users.go`) je tenký: dekóduje
JSON do DTO přes `request.DecodeJSON`, pošle ho přes bus a vrátí odpověď.
Nikdy nevolá handler přímo a neimportuje `infrastructure/`:

```go
// command (bez návratu):
err := bus.ExecVoid(r.Context(), h.commandBus.Bus, "CreateUser", cmd,
    func(ctx context.Context) error { return h.createUser.Handle(ctx, cmd) })
// query (typovaný návrat):
users, err := bus.Exec(r.Context(), h.queryBus.Bus, "ListUsers", q,
    func(ctx context.Context) ([]user.User, error) { return h.listUsers.Handle(ctx, q) })
```

**Route** se registruje v `Server.registerRoutes()` v
`app/presentation/http/server/server.go`. Chráněné a admin routy se obalí
`authed := middleware.AuthMiddleware(s.jwt)`; oddělení user/admin řeší bus
`AuthorizeMiddleware` přes permission, **ne** žádný role-guard middleware:

```go
mux.Handle("POST /api/v1/admin/users", authed(http.HandlerFunc(s.adminUsers.Create)))
```

## Recipe

Přidání nové akce nad **existující** entitou (např. další command):

1. **Repo (když chybí metoda)** — přidej metodu do
   `app/infrastructure/sqlite/<ctx>/repository.go` (vždy přes `r.Conn(ctx)`),
   a její podpis do `Repository` interface v `app/domain/<ctx>/`.
2. **Command / Query** — nový soubor v `app/application/<ctx>/command/`
   (zápis) nebo `…/query/` (čtení): struct, `RequiredPermission()` **nebo**
   `SkipPermission()`, `NewXxxHandler(...)` constructor, `Handle(ctx, …)`.
3. **Handler** — přidej metodu na existující handler v
   `app/presentation/http/handler/` (DTO + `request.DecodeJSON` + `bus.ExecVoid`/
   `bus.Exec`). Nový handler = nový soubor + constructor + pole na `Server`.
4. **Route** — zaregistruj v `registerRoutes()` v `server.go` (obal `authed(...)`
   u chráněných/admin).
5. **DI** (`app/infrastructure/di/container_provider.go`):
   - přidej `NewXxxHandler` do `wire.Build(...)`;
   - **permissioned command/query přidej do `providePermissionsRegistry()`**
     (aby se permission vystavila frontendu; autorizace funguje i bez toho,
     protože `AuthorizeMiddleware` čte permission přímo z `RequiredPermission()`);
   - nový repo = `wire.Bind(new(<ctx>.Repository), new(*sqlite<ctx>.Repository))`
     + `sqlite<ctx>.NewRepository` do `wire.Build`.
6. **`make di && make arch-check`** — vygeneruje `wire_gen.go` a ověří, že
   žádný import neporušil pravidla vrstev. Pak `make lint && make test`.

Pro **nový bounded context** navíc edituj `.go-arch-lint.yml`: přidej
`domain_<ctx>` komponentu, povol ji v `mayDependOn` u každého konzumenta
(`application`, `sqlite_repos`, …) a přidej `infrastructure/sqlite/<ctx>/**`
do `sqlite_repos`. Detail: `/gk-architecture`.

## Invariants & pitfalls

- **Každý command/query MUSÍ deklarovat permission** — `RequiredPermission()`
  nebo explicitní `SkipPermission()`. Zapomenutí obojího = runtime error z
  `AuthorizeMiddleware`. A permissioned command/query patří i do
  `providePermissionsRegistry()`.
- **Bus dispatch je povinný** — z handleru nikdy nevolej `Handle(...)` přímo.
  Bus dodává recovery, logging, autorizaci, transakci a dispatch eventů.
- **Handler neimportuje `infrastructure/`** — jen `application/`, `domain/`,
  `request`, `response`.
- **V repozitáři vždy `r.Conn(ctx)`** (z embedovaného `BaseRepository`), aby se
  zápis účastnil transakce. Výjimky (raw pool) jen tam, kde to musí přežít
  rollback — viz `/gk-database`.
- **Žádné cross-context importy** — `domain/user/` nesmí importovat `domain/token/`.
  Sdílené typy patří do `domain/shared/`.
- **`make di` po každé DI změně** — bez něj se nový handler/provider nepropojí
  a build běží na starém `wire_gen.go`.
- Pozor: `CLAUDE.md` v některých příkladech ukazuje plochou cestu
  `application/command/` — reálný repo má `application/<ctx>/command/`. Drž se
  reálné struktury.

## Related

- Skills: `/gk-domain`, `/gk-bus`, `/gk-handlers`, `/gk-config`, `/gk-architecture`,
  `/gk-database`
- Docs: `CLAUDE.md` (Adding a New Feature)
- Kód: `app/domain/user/`, `app/infrastructure/sqlite/user/repository.go`,
  `app/application/user/command/`, `app/application/user/query/`,
  `app/presentation/http/handler/admin_users.go`,
  `app/presentation/http/server/server.go`,
  `app/infrastructure/di/container_provider.go`
