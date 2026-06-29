---
layout: 'page'
uri: '/framework/job-flow'
position: 70
slug: 'framework-job-flow'
parent: 'framework'
navTitle: 'Job flow'
title: 'Job flow'
description: 'Fire-and-forget tvar durable enginu — krátká background práce, která se zařadí do fronty v transakci commandu (outbox), běží MIMO transakci a doručuje se at-least-once (handler musí být idempotentní). Job je run bez checkpointu.'
---

# Job flow

**Job** je krátký, „udělej a zapomeň" tvar [durable enginu](/framework/run-flow): práce,
která musí proběhnout i po pádu procesu, ale **nepotřebuje si pamatovat postup** (žádný
checkpoint). Hodí se na **volání ven** (e-mail/SMTP, webhook, jedno volání cizího API) i na
rychlý přepočet nad vlastní DB. Job se zapíše do tabulky `runs` **uvnitř transakce commandu**
(outbox), worker si ho atomicky převezme a spustí **mimo transakci**. Selhání se přeplánuje
s backoffem, po vyčerpání pokusů job skončí terminálně.

> ℹ️ Job a [run](/framework/run-flow) jsou **jeden engine** (tabulka `runs`, jeden worker,
> stejné lease/heartbeat). Liší se jen registrací: job = `FireAndForget` (bez checkpointu),
> run = `Durable` (checkpoint + resume). Job je run bez deníku. Kdy co → [Background work](/framework/background-work).

> ⚠️ **Pozor:** handler běží **MIMO transakci** (jako každý run) — i krátké volání ven v
> transakci by drželo SQLite write-lock po dobu volání (SMTP visí, API i minuty) → zamrzne
> celá DB. Atomicitu „práce + hotovo" tu nahrazuje **idempotence**: dokončení se zapisuje
> **samostatně až po návratu handleru**, takže crash mezi tím handler na reclaimu zopakuje.

> Přehled toku. Návod (job kind, handler, retry policy) → `/gk-runs`; periodické úlohy → `/gk-scheduler`.


## K čemu to je

Když práce **musí přežít restart i pád procesu**, nesmí blokovat request, ale **nepotřebuje
resumovat** dosavadní postup (na to je [run](/framework/run-flow)). Zápis do fronty uvnitř
transakce commandu (outbox pattern) dá záruku: job se neztratí (commit ho zviditelní) ani
neosiří (rollback ho vrátí zpět). Cenou je doručení **at-least-once** — handler musí být
idempotentní, a to **včetně svých DB zápisů** (mimo transakci už je žádný in-tx rollback
neochrání).


## Jak to teče

1. **Enqueue** — handler volá `RunDispatcher.Enqueue(kind, maxRetries, payload)`; INSERT
   proběhne přes `r.Conn(ctx)`, takže se připojí ke **stejné transakci** jako business zápis.
2. **Commit** → job je viditelný pro worker. **Rollback** → žádný osiřelý job.
3. **Claim** — worker se každou chvíli ptá fronty a atomicky si převezme (claim) job, který
   je na řadě, jediným `UPDATE … RETURNING` (`attempts++`, nastaví lease). Dva workery
   nedostanou stejný řádek.
4. **Execution** — handler dostane payload a běží **MIMO transakci** (worker mu ctx označí
   `ContextForbidTx`, takže `BeginTx` fail-closed selže). Při úspěchu worker zapíše
   `MarkComplete` **samostatným zápisem** po návratu handleru.
5. **Retry** — chyba nebo panika → `attempts ≤ maxRetries` → `Reschedule` s exponenciálním
   backoffem; jinak `MarkFailed` + **report do Sentry**.

Stav se odvozuje ze sloupců (`completed_at` / `failed_at` / `locked_until`), žádný `status`
enum tu není.


## Příklad

```go
// registrace (provideRunHandlerRegistry, DI) — job = FireAndForget, bez checkpointu:
"welcome:send": runapp.FireAndForget(h.SendWelcome, 30*time.Second),

// z command (nebo event) handleru:
shared.RunDispatcherFromContext(ctx).Enqueue(ctx, "welcome:send", maxRetries, payload)
```

Neznámý `kind` (bez registrovaného handleru) i `maxRetries < 0` selžou už při zařazení do
fronty. Mimo bus (CLI, testy) je dispatcher no-op, takže volání je vždy bezpečné.


## Související

- [Run flow](/framework/run-flow) — durable tvar téhož enginu (checkpoint + resume po pádu).
- [Command flow](/framework/command-flow) — transakce, ke které se zařazení do fronty připojí.
- [Event flow](/framework/event-flow) — odkud se často zařazuje navazující práce.
- Skilly: `/gk-runs`, `/gk-scheduler`, `/gk-di`.
