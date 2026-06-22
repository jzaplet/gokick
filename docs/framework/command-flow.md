---
layout: 'page'
uri: '/framework/command-flow'
position: 50
slug: 'framework-command-flow'
parent: 'framework'
navTitle: 'Command flow'
title: 'Command flow'
description: 'Zápisová cesta CQRS — middleware chain CommandBusu od HTTP handleru po commit a rozeslání eventů.'
---

# Command flow

Zápisové operace tečou přes `CommandBus`. HTTP handler nevolá command handler přímo — pošle ho přes bus, který kolem handleru obalí pevný řetězec middleware: recovery, logging, autorizaci, resoluci tenanta, audit, vložení job dispatcheru, sběr eventů a transakci. Handler se tak stará jen o aplikační logiku.

> Přehled toku. Návody: dispatch a chain `/gk-bus`, psaní handlerů `/gk-commands`, audit `/gk-audit`, eventy `/gk-domain-events`.


## K čemu to je

Pro operace, které **mění stav** (vytvoř / uprav / smaž). Bus kolem handleru obstará vše průřezové (cross-cutting): autorizaci, audit bezpečnostních akcí, atomickou transakci a rozeslání doménových eventů až po commitu. Handler se stará jen o validaci a volání repozitářů.


## Jak to teče

Chain se sestaví jednou v `provideCommandBus`. Pořadí zvenku dovnitř:

1. **Recovery** — panika → `PanicError` (500) + log + report.
2. **Logging** — `bus: executing` → `completed` / `failed` + `duration_ms`.
3. **Authorize** — command musí být `Permissioned` (ověří se permission), nebo `SkipPermission`; jinak chyba.
4. **Tenant** — resolvuje aktivní tenant do `ctx` (z JWT claimu), aby ho repozitáře viděly (row-level multitenancy; viz `/gk-multitenancy`).
5. **Audit** — vloží `AuditCollector` do `ctx`, po handleru zapíše záznamy.
6. **JobDispatcher** — vloží `JobDispatcher` do `ctx` (handler může zařazovat joby do fronty).
7. **DispatchEvents** — vloží per-request `EventCollector`; po úspěšném commitu eventy synchronně rozešle.
8. **Transaction** — `BeginTx` → handler → `Commit` při úspěchu, `Rollback` při chybě.

Uvnitř pak běží command handler: validace přes value objects, `repo.Save`, `Collect(event)`, `Record(audit)`.

Dvě věci stojí za zapamatování: **Audit je mimo transakci** (bezpečnostní záznam přežije rollback, zapisuje se přes raw pool) a **DispatchEvents obaluje transakci** — eventy se rozešlou až po commitu, při rollbacku se zahodí. `Enqueue` jobu naopak běží uvnitř transakce: business zápis i job se uloží atomicky.


## Příklad

```go
err := bus.ExecVoid(r.Context(), h.commandBus.Bus, "CreateUser", cmd,
    func(ctx context.Context) error {
        return h.createUser.Handle(ctx, cmd)
    },
)
if err != nil {
    response.HandleError(w, err)
    return
}
```

`ExecVoid` je `Exec[any]` bez návratové hodnoty; `Exec[R]` použij, když command něco vrací.


## Související

- [Query flow](/framework/query-flow) — čtecí cesta (jen Recovery → Logging → Authorize → Tenant).
- [Event flow](/framework/event-flow) — co se děje po commitu v `EventBus`.
- [Request flow](/framework/request-flow) — HTTP chain před busem.
- Skilly: `/gk-bus`, `/gk-commands`, `/gk-audit`, `/gk-domain-events`.
