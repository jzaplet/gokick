---
layout: 'page'
uri: '/framework/job-flow'
position: 70
slug: 'framework-job-flow'
parent: 'framework'
navTitle: 'Job flow'
title: 'Job flow'
description: 'Cesta perzistentního background jobu — zápis do fronty v transakci commandu (outbox), atomické převzetí workerem, retry s backoffem a at-least-once doručení.'
---

# Job flow

Perzistentní fronta pro práci, která musí proběhnout i po pádu procesu (poslat mail, volat externí API). Job se zapíše do tabulky `jobs` **uvnitř transakce commandu** (outbox), worker si ho atomicky převezme, spustí handler a označí dokončení **ve stejné transakci**. Selhání se přeplánuje s backoffem, po vyčerpání pokusů job skončí terminálně.

> Přehled toku. Návod (job kind, handler, retry policy) → `/gk-jobs`; periodické úlohy → `/gk-scheduler`.


## K čemu to je

Když práce **musí přežít restart i pád procesu** a nesmí blokovat request. Zápis do fronty uvnitř transakce commandu (outbox pattern) dá záruku: job se neztratí (commit ho zviditelní) ani neosiří (rollback ho vrátí zpět). Cenou je doručení **at-least-once** — handler musí být idempotentní.


## Jak to teče

1. **Enqueue** — handler volá `JobDispatcher.Enqueue(kind, maxRetries, payload)`; INSERT proběhne přes `r.Conn(ctx)`, takže se připojí ke **stejné transakci** jako business zápis.
2. **Commit** → job je viditelný pro worker. **Rollback** → žádný osiřelý job.
3. **Claim** — worker se každou `1s` ptá fronty a atomicky si převezme (claim) job, který je na řadě, jediným `UPDATE … RETURNING` (`attempts++`, `locked_until = now + 5m`). Dva workery nedostanou stejný řádek.
4. **Execution** — `runWithinTx`: handler dostane payload a při úspěchu `MarkComplete` **ve stejné transakci** → `COMMIT` (zápisy do DB i dokončení jobu atomicky).
5. **Retry** — chyba nebo panika → rollback. `attempts ≤ maxRetries` → `Reschedule` s exponenciálním backoffem (`2^(n-1)·5s`, cap `1h`); jinak `MarkFailed` + **report do Sentry**.

Stav se odvozuje ze sloupců (`completed_at` / `failed_at` / `locked_until`), žádný `status` enum tu není.


## Příklad

```go
// z command (nebo event) handleru:
shared.JobDispatcherFromContext(ctx).Enqueue(ctx, kind, maxRetries, payload)
```

Neznámý `kind` (bez registrovaného handleru) i `maxRetries < 0` selžou už při zařazení do fronty. Mimo bus (CLI, testy) je dispatcher no-op, takže volání je vždy bezpečné.


## Související

- [Command flow](/framework/command-flow) — transakce, ke které se zařazení do fronty připojí.
- [Event flow](/framework/event-flow) — odkud se často zařazuje navazující práce.
- Skilly: `/gk-jobs`, `/gk-scheduler`, `/gk-di`.
