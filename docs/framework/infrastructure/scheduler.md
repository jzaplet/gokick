---
layout: 'page'
uri: '/framework/infrastructure/scheduler'
position: 5
slug: 'framework-infrastructure-scheduler'
parent: 'framework-infrastructure'
navTitle: 'Scheduler'
title: 'Scheduler'
description: 'Balíček infrastructure/scheduler/ -- in-process periodické joby s graceful shutdown.'
---

# Scheduler


## Proč

Periodické maintenance úlohy (cleanup expirovaných tokenů, cache warming, heartbeats) bez externího cronu. Žije v paměti `serve` procesu a sdílí jeho lifecycle — jeden SIGTERM drainuje scheduler i HTTP server. Pro práci, která musí přežít restart (emaily, externí API), použij [Job Queue](/framework/infrastructure/job-queue).


## Jak

`Job` je trojice `{Name, Interval, Fn}`. `Scheduler` spustí každý job ve vlastní goroutině; `ctx.Done()` ukončí všechny.

```go
// infrastructure/scheduler/scheduler.go

type Job struct {
    Name     string
    Interval time.Duration
    Fn       func(ctx context.Context) error
}

func NewScheduler(logger *slog.Logger, jobs []Job) (*Scheduler, error)
func (s *Scheduler) Run(ctx context.Context)
```

Joby se registrují v jednom místě — `provideSchedulerJobs` v `infrastructure/di/container_provider.go`, stejný pattern jako [`providePermissionsRegistry`](/guides/permissions):

```go
func provideSchedulerJobs(tokens token.TokenRepository) []scheduler.Job {
    return []scheduler.Job{
        {Name: "cleanup:expired-refresh-tokens", Interval: time.Hour, Fn: tokens.DeleteExpired},
    }
}
```

`ServeCommand` spustí `scheduler.Run(ctx)` v goroutině před `server.Start(ctx)`; sdílený `ctx` zaručí drain obojího jedním SIGTERM. Detail v [Console](/framework/presentation/console).


## Detaily

- **Run-once-then-tick** — `Fn` proběhne hned po startu, pak každých `Interval`. Frekventně restartovaný proces (deploys) stále dostane minimálně jeden tick.
- **Panic recovery per tick** — panic v jednom jobu se zaloguje, další tick proběhne, sourozenecké joby pokračují.
- **Error nezhasí ticker** — chyba z `Fn` se zaloguje na ERROR, ticker tiká dál (idempotentní semantika).
- **Validace v constructoru** — duplicitní `Name`, nulový `Interval`, nil `Fn` → error z `NewScheduler` → Wire fail-fast při `CreateApplication`.
- **In-process, ne cross-instance** — dvě repliky tickají nezávisle. Pro multi-instance koordinaci přidat DB lock nebo přejít na centrální scheduler.
- **Joby nesmí předpokládat HTTP ready** — scheduler i `server.Start` jedou paralelně. DB/repository volání jsou OK (Wire init je hotový), self-call přes HTTP je závod.
