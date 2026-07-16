---
layout: 'page'
uri: '/skills/gk-queries'
position: 30
slug: 'skills-gk-queries'
parent: 'skills-cqrs'
navTitle: 'gk-queries'
title: 'GK — Query handlers (read operace)'
description: 'Read operace (čtení dat bez změny stavu) — struktura query handleru, deklarace permission a typovaný návrat přes bus.Query. Use when přidáváš endpoint, který něco čte/vypisuje (list, detail, dashboard), a řešíš strukturu handleru, permission nebo jak ho poslat přes bus.'
name: 'gk-queries'
---

# GK — Query handlers (read operace)

Query čte stav systému a **nic nemění**. V gokicku má pevnou strukturu, deklaruje
permission a posílá se přes `QueryBus` typovaným `bus.Query`.

## What & when

- Sáhni sem, když přidáváš čtecí endpoint: výpis (`ListUsers`), detail, dashboard —
  cokoli, co jen vrací data a nezapisuje do DB.
- **Netýká se zápisu.** Cokoli, co mění stav (create/update/delete), je *command* →
  `/gk-bus`. Rozdíl: command jde přes transakci + audit + eventy, query ne.
- Pokud řešíš celý průchod busem a middleware → `/gk-bus`. Pokud přidáváš featuru
  napříč vrstvami → `/gk-feature`.

## For non-tech / juniors

Představ si knihovnu. **Query** je „přijdu k pultu a zeptám se: ukaž mi seznam knih"
— jen čtu, nic nepřesouvám. **Command** je „chci si knihu půjčit" — mění se stav
(kniha je teď u mě). Gokick to dělí schválně (vzor zvaný **CQRS** — Command Query
Responsibility Segregation): čtení a zápis mají jiná pravidla, takže je drží
oddělené. Čtení nepotřebuje transakci ani záznam do audit logu — jen ověří, že na to
máš oprávnění (permission), a vrátí data.

## How it works

Query žije v `app/application/<context>/query/` — např.
`app/application/user/query/list_users.go`,
`app/application/dashboard/query/get_admin_dashboard.go`.

**Tři části** (viz `list_users.go`):

```go
// 1) Query struct — drží parametry tak, jak přišly po drátě (syrové stringy).
type ListUsersQuery struct {
    Page     int
    PerPage  int
    SortBy   string
    SortDir  string
    Nickname string
    Email    string
    Role     string
    Active   string
}

// 2) Permission — POVINNÉ (viz Invariants).
func (ListUsersQuery) RequiredPermission() string { return "admin:users:read" }

// 3) Handler — konstruktor bere DOMÉNOVÉ rozhraní, ne konkrétní repo.
type ListUsersHandler struct{ users user.Repository }
func NewListUsersHandler(users user.Repository) *ListUsersHandler {
    return &ListUsersHandler{users: users}
}
func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsersQuery) (user.ListPage, error) {
    // Handle syrový vstup normalizuje do whitelistovaných doménových kritérií:
    // neznámý sloupec/směr a stránka mimo rozsah spadnou na default, ne na 400
    // — řazení a stránkování jsou UX preference, ne kontrakt.
    criteria := user.ListCriteria{
        Page:    q.Page,
        PerPage: q.PerPage,
        Sort:    user.SortColumnFrom(q.SortBy),
        SortDir: shared.SortDirectionFrom(q.SortDir),
        Filters: user.ListFilters{
            Nickname: q.Nickname,
            Email:    q.Email,
            Role:     q.Role,
            Active:   q.Active,
        },
    }.Normalize()

    return h.users.FindPage(ctx, criteria)
}
```

**Návratový typ je libovolný** — stránka dat (`user.ListPage`, tj. `Items` + `Total`)
i vlastní DTO struct (`AdminDashboard{ UsersActive, UsersTotal int }`
v `get_admin_dashboard.go`).

**Dispatch z HTTP handleru** přes generický `bus.Query[R]`
(`app/application/bus/dispatch.go`) — `R` je typ výsledku, takže návrat je typovaný.
`bus.Query` bere přímo `*bus.QueryBus` (vnitřní `*Bus` je neexportovaný), takže párování
bus↔operace hlídá kompilátor — query nejde omylem poslat na command bus:

```go
// app/presentation/http/handler/dashboard.go
result, err := bus.Query(
    r.Context(),
    h.queryBus,                 // *bus.QueryBus
    "GetAdminDashboard",        // jméno do logů
    q,                          // samotná query (nese permission)
    func(ctx context.Context) (dashboardqry.AdminDashboard, error) {
        return h.adminDash.Handle(ctx, q)
    },
)
```

```go
// app/presentation/http/handler/dashboard.go
result, err := bus.Exec(
    r.Context(),
    h.queryBus.Bus,            // *bus.QueryBus
    "GetAdminDashboard",        // jméno do logů
    q,                          // samotná query (nese permission)
    func(ctx context.Context) (dashboardqry.AdminDashboard, error) {
        return h.adminDash.Handle(ctx, q)
    },
)
```

**QueryBus middleware chain** je krátký a jen čtecí — `BaseChain` v
`app/application/bus/middleware/base.go`:

```
Recovery → Logging → Authorize → Tenant
```

`Tenant` (`middleware/tenant.go`) hned po autorizaci resolvuje aktivní tenant do ctx — čtení potřebuje tenant scoping stejně jako zápis (viz `/gk-multitenancy`).

Žádná transakce, žádný audit, žádné eventy (to mají jen command busy — `CommandBus` a `SystemCommandBus`). `Authorize`
(`middleware/authorize.go`) zavolá `RequiredPermission()` a ověří ho proti rolím
volajícího.

## Recipe

Přidání nové query (čtecí endpoint):

1. **Soubor** `app/application/<context>/query/<verb>_<noun>.go` (např.
   `user/query/list_users.go`).
2. **Query struct** s filtry (klidně prázdný `struct{}`).
3. **Permission**: přidej `RequiredPermission() string` (vrať existující permission
   string), nebo u veřejné query `func (Q) SkipPermissionCheck() {}`.
4. **Handler** + `NewXxxHandler(...)` konstruktor; závislosti ber jako **doménová
   rozhraní** (`user.Repository`), ne konkrétní `*sqliteuser.Repository`.
5. **`Handle(ctx, q) (R, error)`** — jen čte, žádné zápisy.
6. **HTTP handler** v `presentation/http/handler/` dispatchni přes
   `bus.Query[R](ctx, h.queryBus, "Name", q, fn)`; výsledek namapuj na DTO a vrať
   `h.resp.JSON(r.Context(), w, http.StatusOK, dto)`, chybu přes `h.resp.HandleError(r.Context(), w, err)` (`resp *response.Responder` si handler nechá injektovat v konstruktoru).
7. **Route** zaregistruj v `presentation/http/server/server.go`, **DI** dráty
   (provider handleru + query handleru) v `infrastructure/di/container_provider.go`,
   pak `make di`. Celý průchod vrstvami → `/gk-feature`.

## Invariants & pitfalls

- **Permission je povinná.** Každá query MUSÍ implementovat buď `Permissioned`
  (`RequiredPermission`), nebo `SkipPermission` (`SkipPermissionCheck`). Když chybí
  obojí, `AuthorizeMiddleware` vrátí error (`bus: command %q must implement …`) —
  chrání před zapomenutou deklarací.
- **Query nemá side-effects.** Jen čte. Žádné `Save`/zápis do DB, žádné sbírání
  doménových eventů (`EventCollector` patří commandům). Pokud potřebuješ zápis, je to
  command, ne query.
- **Závislost na doménovém rozhraní, ne na konkrétním repu.** Handler drží
  `user.Repository`, ne `*sqliteuser.Repository` — jinak porušíš pravidlo vrstev
  (`make arch-check` to chytne).
- **Nikdy nevolej handler napřímo z HTTP** — vždy přes `bus.Query`. Mimo bus by
  permission check ani logging neproběhly.
- **Žádné raw permission stringy na frontendu.** Backendová query deklaruje string
  (`admin:users:read`), FE musí stejnou permission brát z `Permission` enumu
  (`assets/app/Auth/enums/resources.ts`) — viz `CLAUDE.md`.

## Related

- Skills: `/gk-bus` (busy + middleware chain + dispatch), `/gk-feature` (query
  end-to-end přes vrstvy), `/gk-entities` (entity a value objects, které query vrací)
- Kód: `app/application/user/query/list_users.go`,
  `app/application/dashboard/query/get_admin_dashboard.go`,
  `app/application/bus/dispatch.go`, `app/application/bus/middleware/base.go`,
  `app/presentation/http/handler/dashboard.go`
