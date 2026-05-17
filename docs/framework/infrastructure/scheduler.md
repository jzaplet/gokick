---
layout: 'page'
uri: '/framework/infrastructure/scheduler'
position: 5
slug: 'framework-infrastructure-scheduler'
parent: 'framework-infrastructure'
navTitle: 'Scheduler'
title: 'Scheduler'
description: 'In-process scheduler pro periodické úlohy -- run-once-then-tick, panic recovery, graceful shutdown s HTTP serverem.'
---

# Scheduler

In-process scheduler pro periodické úlohy (cron-like) — refresh token cleanup, statistiky, kterékoli "udělej X každých Y". Žádný externí cron, žádný DB-backed scheduler; žije v paměti `serve` procesu, sdílí jeho lifecycle.


## Proč

- **Single-binary deploy** — žádný extra orchestrator pro úlohy typu „smaž expired tokeny každou hodinu".
- **Lifecycle ruku v ruce s HTTP serverem** — stejný `ctx` z `signal.NotifyContext` drainuje scheduler i server jedním SIGTERM; žádný leak goroutin při restartu.
- **Run-once-then-tick semantika** — maintenance úloha proběhne ihned po startu (ne až za interval), takže frekventně restartovaný proces (deploys, dev) stejně garantuje minimálně jeden cleanup za lifetime.

Pokud potřebujete úlohu, která **musí přežít restart** (welcome maily, externí API), patří do perzistentní [job queue (roadmap F3)](/roadmap#fáze-3--perzistentní-job-queue-sqlite), ne sem.


## Jak

### Job a Scheduler

Žije v `infrastructure/scheduler/scheduler.go`:

```go
type JobFunc func(ctx context.Context) error

type Job struct {
    Name     string         // unikátní, hláška v logu
    Interval time.Duration  // mezi tikami
    Fn       JobFunc         // co se má provést
}

func NewScheduler(logger *slog.Logger, jobs []Job) (*Scheduler, error)
func (s *Scheduler) Run(ctx context.Context)
```

`NewScheduler` validuje při startu:
- nepřázdné `Name`,
- pozitivní `Interval`,
- non-nil `Fn`,
- žádné duplicitní `Name`.

Selhání validace = `error` z constructoru = Wire DI zase z `CreateApplication` = proces ani nestartuje (fail-fast).

### Registrace jobu

V `infrastructure/di/container_provider.go`:

```go
func provideScheduler(logger *slog.Logger, tokens token.TokenRepository) (*scheduler.Scheduler, error) {
    return scheduler.NewScheduler(logger, []scheduler.Job{
        {
            Name:     "cleanup:expired-refresh-tokens",
            Interval: 1 * time.Hour,
            Fn:       tokens.DeleteExpired,
        },
        // přidej další úlohy sem
    })
}
```

Hardcodovaný seznam je záměrný — stejný pattern jako [PermissionsRegistry](/guides/permissions). Při ~3+ úlohách se vyplatí registry pattern (analogie permissions), ale pro jednu/dvě úlohy je inline list jasnější.

### Lifecycle v `ServeCommand`

```go
RunE: func(cmd *cobra.Command, _ []string) error {
    ctx := cmd.Context()

    schedulerDone := make(chan struct{})
    go func() {
        defer close(schedulerDone)
        c.scheduler.Run(ctx)
    }()

    serverErr := c.server.Start(ctx)
    <-schedulerDone
    return serverErr
}
```

Scheduler i server sdílí ten samý `ctx` (z `signal.NotifyContext` v `main.go`). SIGTERM → ctx cancel → scheduler drainuje běžící tick → `server.Start` drainuje inflight HTTP requesty → `RunE` vrátí.

### Per-job běh

`runJob` v scheduleru:

1. **Tick #1** ihned (run-once-then-tick).
2. `time.NewTicker(Interval)` — každý další tick.
3. Před každým tickem `defer recover()` — panic v jednom jobu se zaloguje a další tick proběhne normálně. Sourozenecké joby nejsou ovlivněny.
4. Chyba z `Fn` se zaloguje na ERROR, ale neukončí job. Další tick proběhne.

`ctx.Done()` v select preempuje ticker → goroutina exitne, scheduler drainuje WaitGroup.


## Detaily

### Co je dobré dát do scheduleru

- DB cleanup (expired rows, stale sessions, log retention).
- Cache warming z DB.
- Heartbeats / health metrics emisia.
- Cokoli, co je idempotentní a kde ztráta jednoho ticku není kritická.

### Co tam nepatří

- **Práce co musí přežít restart** — emaily, externí API, retry-prone I/O. Patří do [job queue (F3)](/roadmap#fáze-3--perzistentní-job-queue-sqlite).
- **Cross-instance koordinace** — scheduler je in-process, dvě repliky odpálí cleanup 2×. Pro multi-instance scaling se musí přidat DB lock nebo přejít na centrální scheduler.
- **Sub-sekundový tick** — `time.Ticker` zvládne, ale výkonově je to nesmysl. Pro reaktivní logiku použijte event handler nebo HTTP webhook.
- **Joby závislé na HTTP server ready stavu** — scheduler i `server.Start` jedou paralelně. První tick (run-once) typicky proběhne dřív, než server přijme první connection. Job nesmí předpokládat, že vlastní HTTP API už odpovídá. Pro DB / repository volání je to OK (Wire DI je inicializované ještě před `Run`), pro self-call je to závod.

### Cleanup expired refresh tokens — design notes

`token.TokenRepository.DeleteExpired` je `DELETE FROM refresh_tokens WHERE expires_at < datetime('now')`. Jednorázový statement, atomický, žádná transakce nutná. Job ho volá s `ctx` ze scheduleru (timeout = scheduler shutdown).

Pozor: `used_at` se **nepoužívá** v kritériu. Token, který byl rotated (used_at != nil), zůstává v DB do své `expires_at` — slouží theft-detection okénku. Smazat ho dřív by ztratilo signál „theft" vs „stale" bez bezpečnostního přínosu.

### Vstup od `provideScheduler` do Wire

Wire reportuje `unused provider`, pokud `*scheduler.Scheduler` nikdo neinjektuje. `ServeCommand` ho dnes drží jako jediný — `seed` a `create-user` Cobra příkazy scheduler nedostávají (a nepotřebují, vlastní lifecycle).

### Testy

Tabulka v `app/infrastructure/scheduler/scheduler_test.go`:

| Test | Co ověřuje |
|---|---|
| `TestScheduler_RunsAndStops` | Krátký interval + counter + cancel: každý job tikne aspoň 2× (run-once + tick) a drainuje se |
| `TestNewScheduler_DuplicateName` | Duplicitní jméno → error z constructoru |
| `TestNewScheduler_InvalidJob` | Prázdné jméno / nulový interval / nil Fn → error |
| `TestScheduler_PanicInOneJobKeepsOthersRunning` | Panicující job se zotaví, sourozenecké joby ticky pokračují |
| `TestScheduler_ErrorReturnedJobKeepsTicking` | Job vracející error tikne dál — error je logged, ne fatal |
| `TestScheduler_ImmediateCancelDoesNotHang` | Ctx cancel mezi ticky neblokuje drain |


## Kam dál

| Téma | Odkaz |
|---|---|
| Graceful shutdown HTTP serveru | [HTTP Server](/framework/presentation/http-server) |
| Cobra ServeCommand | [Console](/framework/presentation/console) |
| Wire DI providers | [Wire DI](/framework/infrastructure/wire) |
| Perzistentní job queue (F3) | [Roadmap F3](/roadmap#fáze-3--perzistentní-job-queue-sqlite) |
