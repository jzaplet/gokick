---
layout: 'page'
uri: '/framework/command'
position: 10
slug: 'framework-command'
parent: 'framework-cqrs'
navTitle: 'Command'
title: 'Command'
description: 'Zápisová cesta CQRS — middleware chain CommandBusu od HTTP handleru po commit a rozeslání eventů.'
---

# Command

Zápisové operace tečou přes `CommandBus`. HTTP handler nevolá command handler přímo — pošle ho přes bus, který kolem handleru obalí pevný řetězec middleware: recovery, logging, autorizaci, resoluci tenanta, audit, vložení run dispatcheru, sběr eventů a transakci. Handler se tak stará jen o aplikační logiku.

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
6. **RunDispatcher** — vloží `RunDispatcher` do `ctx` (handler může zařazovat runy do fronty).
7. **DispatchEvents** — vloží per-request `EventCollector`; po úspěšném commitu eventy synchronně rozešle.
8. **Transaction** — `BeginTx` → handler → `Commit` při úspěchu, `Rollback` při chybě. Command se může opt-outnout markerem `shared.SkipsTransaction` (výjimka — raw-pool zápisy u Login, theft-cleanup u RefreshToken; viz `/gk-bus`).

Uvnitř pak běží command handler: validace přes value objects, `repo.Save`, `Collect(event)`, `Record(audit)`.

Dvě věci stojí za zapamatování: **Audit je mimo transakci** (bezpečnostní záznam přežije rollback, zapisuje se přes raw pool) a **DispatchEvents obaluje transakci** — eventy se rozešlou až po commitu, při rollbacku se zahodí. `Enqueue` runu naopak běží uvnitř transakce: business zápis i zařazení do fronty se uloží atomicky (samotný handler pak běží mimo transakci).


## Příklad

```go
err := bus.DispatchVoid(r.Context(), h.commandBus, "CreateUser", cmd,
    func(ctx context.Context) error {
        return h.createUser.Handle(ctx, cmd)
    },
)
if err != nil {
    h.resp.HandleError(r.Context(), w, err)
    return
}
```

`DispatchVoid` je `Dispatch[R]` bez návratové hodnoty; `Dispatch[R]` použij, když command něco vrací. Obě berou `*bus.CommandBus`, takže záměna s QueryBusem nejde přeložit.


## Související

- [Query](/framework/query) — čtecí cesta (jen Recovery → Logging → Authorize → Tenant).
- [Events](/framework/events) — co se děje po commitu v `EventBus`.
- [Request](/framework/request) — HTTP chain před busem.
- Skilly: `/gk-bus`, `/gk-commands`, `/gk-audit`, `/gk-domain-events`.
