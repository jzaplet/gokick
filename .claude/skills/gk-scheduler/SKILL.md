---
layout: 'page'
uri: '/skills/gk-scheduler'
position: 40
slug: 'skills-gk-scheduler'
parent: 'skills-data'
navTitle: 'gk-scheduler'
title: 'GK — In-process scheduler'
description: 'Periodické úlohy uvnitř serveru (cron-like) — goroutina + ticker, první tick hned po startu, panika v jednom jobu neshodí ostatní. Use when chceš spustit něco opakovaně na pozadí (cleanup, sync, housekeeping) bez externího OS cronu.'
name: 'gk-scheduler'
---

# GK — In-process scheduler

Spouštění periodických úloh přímo v `serve` procesu, bez externího cronu. Jeden binary, jeden lifecycle.

## What & when

- Sáhni sem, když potřebuješ něco dělat **opakovaně sám od sebe** — typicky maintenance: smazat expirované tokeny, warmnout cache, emitnout metriku.
- NEtýká se práce, která **musí přežít restart/crash** (welcome maily, volání externího API). To je perzistentní fronta — viz `Related` (durable engine). Scheduler nic neperzistuje; když proces spí, job neběží.

## For non-tech / juniors

Scheduler je „budík" uvnitř aplikace: každých N vteřin/minut/hodin zavolá tvoji funkci. Běží jako součást serveru — **žádný OS cron, žádná druhá služba**. Když nasadíš server, scheduler běží s ním; jeden SIGTERM (signál „ukonči se") ukončí obojí, žádné osamělé běžící úlohy.

Důležitý detail: každý job se spustí **hned po startu** (ne až za hodinu) a pak dál v intervalu. Proč: proces, co se často restartuje (deploye, vývoj), tak má jistotu aspoň jednoho proběhnutí za svůj život.

Když nad jednou databází běží víc kopií serveru (replik), job pustí jen jedna z nich. Ta drží zámek jobu v databázi a ostatní svůj tick přeskočí. Když držitel skončí nebo spadne, job převezme další replika při svém nejbližším ticku.

## How it works

Balíček `app/infrastructure/scheduler/scheduler.go`:

- **`Job`** = `{ Name string, Interval time.Duration, Fn JobFunc }`, kde `JobFunc = func(ctx context.Context) error`.
- **`NewScheduler(logger, locker, jobs) (*Scheduler, error)`** — konstruktor **validuje fail-fast**: chybějící locker, prázdné jméno, `Interval <= 0`, nil `Fn` nebo duplicitní jméno → vrátí error a proces ani nenastartuje.
- **`Run(ctx)`** — spustí každý job ve **vlastní goroutině** (přes `sync.WaitGroup`) a blokuje, dokud se `ctx` nezruší a všechny joby nedoběhnou. Prázdný seznam → jen zaloguje `scheduler: no jobs registered` a vrátí se.
- **`runJob`** — **run-once-then-tick**: zavolá `tick` rovnou jednou, pak založí `time.NewTicker(Interval)` a tiká dokola; `ctx.Done()` smyčku ukončí.
- **Zámek jobu** — `tick` před každým spuštěním podrží zámek `job:<name>` přes `shared.Locker` (port na `persistence.Store`, Wire ho předá do `provideScheduler`). Když zámek drží jiná replika, tick se přeskočí (Debug log). Replika zámek drží, dokud scheduler běží; po zastavení ho `release` uvolní. Když zámek nejde ověřit (DB nedostupná), job se nespustí, zaloguje se Error a další tick to zkusí znovu.
- **`tick`** — jedna invokace `Fn` s **panic recovery**: panika se jen zaloguje na Error úrovni a **NEhlásí se do error trackeru** (na rozdíl od durable-task worker fronty — komentář to v kódu vysvětluje: job re-tiká donekonečna, deterministická panika by jinak generovala nekonečný proud stejných eventů). Vrácený `error` z `Fn` se taky zaloguje a **ticker tiká dál** — předpokládá se idempotence.

**Registrace** (jediné místo) je `provideSchedulerJobs` v `app/infrastructure/di/container_provider.go`. Dnes obsahuje jediný job:

```go
func provideSchedulerJobs(tokens token.Repository) []scheduler.Job {
    return []scheduler.Job{
        {
            Name:     "cleanup:expired-refresh-tokens",
            Interval: 1 * time.Hour,
            Fn:       tokens.DeleteExpired,
        },
    }
}
```

**Spuštění**: `app/presentation/console/serve.go` pustí `scheduler.Run(ctx)` v goroutině, sdílí jeden `ctx` se serverem i durable-task workerem; po pádu/SIGTERM zavolá `cancel()` a počká na `schedulerDone` — drain bez osamělých goroutin.

## Recipe: přidat periodický job

1. **Měj funkci se signaturou `func(ctx context.Context) error`.** Často už existuje na repozitáři (jako `tokens.DeleteExpired`). Musí být **idempotentní** — spustí se hned po startu i po každém restartu.
2. **Přidej `scheduler.Job{Name, Interval, Fn}`** do `provideSchedulerJobs` v `container_provider.go`. `Name` musí být **unikátní**, `Interval > 0`, `Fn != nil`.
3. **`make di`** — přegeneruje Wire (provider je nový/změněný vstup do grafu).
4. **`make serve`** — v logu uvidíš `scheduler: starting` + `scheduler: job completed` (to druhé je run-once tick, proběhlo ihned).

## Invariants & pitfalls

- **Fn běží inline, v žádné transakci.** Nikdo Fn do transakce neobaluje — případnou si musíš otevřít sám. Lehký cleanup v krátké transakci je OK; **těžkou periodickou práci zařaď jako durable run** ([[gk-runs]] — fire-and-forget i durable run), ne ji dělej inline — dlouhá transakce by zamkla SQLite. Kdy co → `docs/framework/background/overview.md`.
- **Fn běží bez tenanta v ctx.** Scheduler žádný tenant neresolvuje — pod `APP_MULTITENANCY=true` job, který sáhne na tenant-owned tabulku (`r.Tenant(ctx)`), zpanikaří při **každém ticku**, a panika se jen zaloguje (do Sentry nejde) — tichý nekonečný fail. Tenant-scoped práce patří do runu (tenant se razítkuje při enqueue a worker ho do ctx vrátí — [[gk-runs]]), nebo si Fn musí tenanty vyřešit explicitně.
- **Jméno unikátní, interval kladný, Fn nenilové.** Jinak `NewScheduler` vrátí error a proces nenastartuje (fail-fast) — chyba se chytí při startu, ne za běhu.
- **Fn musí být idempotentní.** Kvůli run-once-then-tick se job spustí hned a může proběhnout vícekrát; nepředpokládej „přesně jednou".
- **Job nesmí volat vlastní HTTP API přes localhost** — server běží paralelně, vznikl by závod (race). DB volání jsou OK (Wire je v té chvíli hotový).
- **Víc replik: job běží na jedné.** Na Postgresu drží zámek jobu replika, která ho vzala první. Drží ho po celou dobu, co běží (session advisory lock na jednom vyhrazeném spojení systémového poolu, `app/infrastructure/postgres/locker.go`). Zámek jen na dobu běhu jobu by nestačil: repliky by se jen nepřekrývaly a job by dál běžel na každé z nich jednou za interval. Držitel si tak z poolu trvale bere jedno spojení.
- **Převzetí může job zopakovat.** Když držitel spadne těsně po běhu, nový držitel job pustí při svém nejbližším ticku. Job tak může proběhnout dvakrát krátce po sobě, a proto musí zůstat idempotentní.
- **PgBouncer v režimu transaction pooling session zámky rozbije.** Zámek patří serverovému spojení a pooler v tomto režimu serverová spojení pod klientem střídá. Scheduler potřebuje přímé spojení nebo session pooling.
- **Na SQLite zámek nic nevylučuje.** SQLite obsluhuje jeden proces a `sqlite.Locker` povolí každý zámek. Dva `serve` procesy nad jedním souborem by každý job pustily oba.
- **Nedávej sem práci, která musí přežít restart.** Když proces neběží, neběží ani job a nikdo to nedožene. Pro to slouží perzistentní durable engine ([[gk-runs]]).
- **Logování přes injektovaný `*slog.Logger`**, klíče jsou konstanty (`logKeyName`, `logKeyJobs` + sdílené `shared.LogKeyPanic` / `shared.LogKeyStack` / `shared.DurationMsAttr` / `shared.LogKeyError`) — neporušuj jedinou logovací cestu projektu.

## Related

- `/gk-di` — DI registrace providerů a `make di` workflow.
- `/gk-architecture` — proč `scheduler` žije v `infrastructure/` a kdo na něm smí záviset.
- `/gk-runs` — perzistentní durable engine (fire-and-forget i durable run) pro práci, co musí přežít restart/crash.
- Kód: `app/infrastructure/scheduler/scheduler.go`, zámky `app/infrastructure/postgres/locker.go` a `app/infrastructure/sqlite/locker.go`, registrace `app/infrastructure/di/container_provider.go` (`provideSchedulerJobs`/`provideScheduler`), spuštění `app/presentation/console/serve.go`.
