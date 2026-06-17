---
layout: 'page'
uri: '/framework/query-flow'
position: 60
slug: 'framework-query-flow'
parent: 'framework'
navTitle: 'Query flow'
title: 'Query flow'
description: 'Čtecí cesta CQRS — krátký řetězec Recovery → Logging → Authorize, typovaný návrat, žádná transakce ani eventy.'
---

# Query flow

Čtecí operace tečou přes `QueryBus`. Oproti [Command flow](/framework/command-flow) je řetězec krátký — jen **Recovery → Logging → Authorize**. Žádná transakce, audit ani eventy: čtení nemění stav, takže nemá co commitovat ani ohlašovat. Návratová hodnota je typovaná díky generikám v `bus.Exec[R]`.

> Přehled toku. Návod „jak napsat query handler" je ve skillu `/gk-queries`, mechaniku busů rozebírá `/gk-bus`.


## K čemu to je

Když potřebuješ **přečíst data** (seznam, detail) a vrátit je z endpointu. Bus zajistí i u čtení tři věci: panika se nedostane ke klientovi (Recovery), každý dotaz má access log s `duration_ms` (Logging) a permission se vynutí i u čtecího endpointu (Authorize → 403). Co patří k zápisům — transakce, audit, eventy — tu záměrně chybí.


## Jak to teče

1. HTTP handler sestaví query (prostý struct) a pošle ji přes `bus.Exec[R]` — nevolá handler přímo.
2. **Recovery** zachytí případnou paniku → `PanicError` (500) + report.
3. **Logging** zaloguje `bus: executing` → `bus: completed` + `duration_ms`.
4. **Authorize** porovná `RequiredPermission()` s `PermissionChecker` → jinak 403.
5. Query handler `Handle` čte přes `r.Conn(ctx)` (raw pool, žádná transakce).
6. `bus.Exec[R]` vrátí typovaný výsledek (na `nil` vrátí zero value `R`); chybu mapuje `response.HandleError` na HTTP status.


## Příklad

```go
func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
    q := userqry.ListUsersQuery{}

    users, err := bus.Exec(
        r.Context(),
        h.queryBus.Bus,
        "ListUsers",
        q,
        func(ctx context.Context) ([]user.User, error) {
            return h.listUsers.Handle(ctx, q)
        },
    )
    if err != nil {
        response.HandleError(w, err)
        return
    }

    dtos := make([]adminUserDTO, len(users)) // entity → DTO
    for i, u := range users {
        dtos[i] = toAdminUserDTO(u)
    }
    response.JSON(w, http.StatusOK, dtos)
}
```

Query handler drží jen doménový repository interface a **musí** deklarovat permission (`Permissioned`, nebo výslovně `SkipPermission`) — jinak ho `AuthorizeMiddleware` odmítne.


## Související

- [Command flow](/framework/command-flow) — zápisová cesta s transakcí, auditem a eventy.
- [Request flow](/framework/request-flow) — HTTP middleware chain před vstupem do busu.
- [Architecture](/framework/architecture) — vrstvy a CQRS jako celek.
- Skilly: `/gk-queries`, `/gk-bus`, `/gk-permissions`, `/gk-repositories`.
