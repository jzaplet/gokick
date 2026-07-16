---
layout: 'page'
uri: '/framework/query-flow'
position: 60
slug: 'framework-query-flow'
parent: 'framework'
navTitle: 'Query flow'
title: 'Query flow'
description: 'Čtecí cesta CQRS — krátký řetězec Recovery → Logging → Authorize → Tenant, typovaný návrat, žádná transakce ani eventy.'
---

# Query flow

Čtecí operace tečou přes `QueryBus`. Oproti [Command flow](/framework/command-flow) je řetězec krátký — jen **Recovery → Logging → Authorize → Tenant**. Žádná transakce, audit ani eventy: čtení nemění stav, takže nemá co commitovat ani ohlašovat. Tenant resoluce tu je (čtení se scopuje stejně jako zápisy — viz `/gk-multitenancy`). Návratová hodnota je typovaná díky generikám v `bus.Query[R]`.

> Přehled toku. Návod „jak napsat query handler" je ve skillu `/gk-queries`, mechaniku busů rozebírá `/gk-bus`.


## K čemu to je

Když potřebuješ **přečíst data** (seznam, detail) a vrátit je z endpointu. Bus zajistí i u čtení čtyři věci: panika se nedostane ke klientovi (Recovery), každý dotaz má access log s `duration_ms` (Logging), permission se vynutí i u čtecího endpointu (Authorize → 403) a dotaz se scopne na aktivní tenant (Tenant). Co patří k zápisům — transakce, audit, eventy — tu záměrně chybí.


## Jak to teče

1. HTTP handler sestaví query (prostý struct) a pošle ji přes `bus.Query[R]` — nevolá handler přímo.
2. **Recovery** zachytí případnou paniku → `PanicError` (500) + report.
3. **Logging** zaloguje `bus: executing` → `bus: completed` + `duration_ms`.
4. **Authorize** porovná `RequiredPermission()` s `PermissionChecker` → jinak 403.
5. **Tenant** resolvuje aktivní tenant do `ctx` (row-level multitenancy; viz `/gk-multitenancy`).
6. Query handler `Handle` čte přes `r.Conn(ctx)` (raw pool, žádná transakce).
7. `bus.Query[R]` vrátí typovaný výsledek (na `nil` vrátí zero value `R`); chybu mapuje `resp.HandleError` (metoda injektovaného `*response.Responder`) na HTTP status.


## Příklad

```go
func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
    q := listUsersQueryFromRequest(r) // stav gridu (stránka, řazení, filtry) z query stringu

    page, err := bus.Query(
        r.Context(),
        h.queryBus,
        "ListUsers",
        q,
        func(ctx context.Context) (user.ListPage, error) {
            return h.listUsers.Handle(ctx, q)
        },
    )
    if err != nil {
        h.resp.HandleError(r.Context(), w, err)
        return
    }

    dtos := make([]adminUserDTO, len(page.Items)) // entity → DTO
    for i, u := range page.Items {
        dtos[i] = toAdminUserDTO(u)
    }
    h.resp.JSON(r.Context(), w, http.StatusOK, adminUserListResponse{
        Items: dtos,
        Total: page.Total, // celkový počet PŘED stránkováním — grid z něj počítá pager
    })
}
```

Query handler drží jen doménový repository interface a **musí** deklarovat permission (`Permissioned`, nebo výslovně `SkipPermission`) — jinak ho `AuthorizeMiddleware` odmítne.


## Související

- [Command flow](/framework/command-flow) — zápisová cesta s transakcí, auditem a eventy.
- [Request flow](/framework/request-flow) — HTTP middleware chain před vstupem do busu.
- [Architecture](/framework/architecture) — vrstvy a CQRS jako celek.
- Skilly: `/gk-queries`, `/gk-bus`, `/gk-permissions`, `/gk-repositories`.
