---
layout: 'page'
uri: '/framework/scheduler-flow'
position: 80
slug: 'framework-scheduler-flow'
parent: 'framework'
navTitle: 'Scheduler flow'
title: 'Scheduler flow'
description: 'In-process scheduler — periodické joby, každý ve své goroutině, run-once-then-tick, per-tick recovery a drain při SIGTERM.'
---

# Scheduler flow

In-process scheduler pro periodickou (cron-like) práci uvnitř běžícího serveru — žádný externí OS cron, žádný samostatný proces. Každý job běží ve vlastní goroutině, poprvé hned po startu a pak každý `Interval`. Implementace je v `app/infrastructure/scheduler/scheduler.go`.

> Přehled toku. Návod „přidat periodický job" → `/gk-scheduler`; perzistentní fronta → `/gk-jobs`.


## K čemu to je

Na údržbové úlohy uvnitř procesu — úklid, synchronizace, sběr statistik. **Není** to perzistentní fronta: stav je jen v paměti (in-memory), bez retry, restart úlohy jen znovu rozběhne. Práci, která **musí** proběhnout i po pádu procesu, posílej do [Job flow](/framework/job-flow) (`/gk-jobs`).


## Jak to teče

1. `NewScheduler` ověří joby **při startu** (fail-fast): neprázdné jméno, `Interval > 0`, `Fn != nil`, unikátní jména — jinak se aplikace nerozběhne.
2. `Run(ctx)` spustí každý job ve vlastní goroutině.
3. **Run-once-then-tick**: `Fn` se spustí hned, teprve pak se spustí `time.Ticker` — údržba tak proběhne aspoň jednou za životnost procesu, i při častých restartech.
4. Každý `tick` má vlastní `recover()` — panika v jednom jobu ostatní nepoloží. Recovery jen loguje, **nehlásí do Sentry** (deterministická panika by se jinak opakovala při každém ticku donekonečna).
5. SIGTERM zruší sdílený `ctx`, každá goroutina opustí `select`, `Run` blokuje až do `wg.Wait()` — scheduler i server tak doběhnou zároveň.


## Příklad

Joby se registrují na jednom místě — `provideSchedulerJobs` v DI:

```go
scheduler.Job{
    Name:     "cleanup:expired-refresh-tokens",
    Interval: time.Hour,
    Fn:       tokenRepo.DeleteExpired,
}
```

Aktuálně jediný job maže prošlé refresh tokeny (`WHERE datetime(expires_at) < datetime('now')`); díky run-once-then-tick proběhne úklid hned po startu.


## Související

- [Job flow](/framework/job-flow) — perzistentní fronta pro práci, která musí přežít restart.
- [Architecture](/framework/architecture) — vrstvy a startup sekvence.
- Skilly: `/gk-scheduler`, `/gk-jobs`, `/gk-logging`.
