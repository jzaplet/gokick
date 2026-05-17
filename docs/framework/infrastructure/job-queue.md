---
layout: 'page'
uri: '/framework/infrastructure/job-queue'
position: 6
slug: 'framework-infrastructure-job-queue'
parent: 'framework-infrastructure'
navTitle: 'Job Queue'
title: 'Job Queue'
description: 'Perzistentní job queue v SQLite -- background práce co musí přežít restart, mark-complete-in-handler-tx.'
---

# Job Queue


## Proč

Background práce, která **musí přežít restart procesu nebo crash**: odesílání emailů, externí API volání, retry-prone I/O. Pro periodické maintenance bez perzistence patří úloha do [Scheduleru](/framework/infrastructure/scheduler).

Tři kontrakty:

1. **Atomicita business write + enqueue.** Command handler může zapsat uživatele a zaregistrovat welcome job ve stejné transakci.
2. **At-least-once delivery.** Handler může běžet víc než jednou (crash, commit failure). Handlery musí být idempotentní pro **externí** side effects.
3. **Mark-complete v handler's tx.** Handler success + MarkComplete commitují atomicky; handler-fail rollbackne i handler's DB writes. Žádné částečné stavy.


## Jak

Tři komponenty:

| Vrstva | Soubor | Co dělá |
|---|---|---|
| `job.Repository` | `domain/job/`, `infrastructure/sqlite/job/` | Atomický `UPDATE … RETURNING` claim, MarkComplete, Reschedule, MarkFailed |
| `shared.JobDispatcher` | `domain/shared/job_dispatcher.go` + `application/job/dispatcher.go` | Enqueue z handleru přes ctx (`JobDispatcherFromContext`) |
| `worker.Worker` | `infrastructure/worker/worker.go` | Pool goroutin, claim, run-in-tx, mark-complete, backoff |

### Enqueue z command handleru

`JobDispatcherMiddleware` vloží dispatcher do ctx ještě před `TransactionMiddleware`, takže `Enqueue` se připojí do business transakce — buď oboje commitne, nebo nic.

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    if err := h.users.Save(ctx, u); err != nil {
        return err
    }
    return shared.JobDispatcherFromContext(ctx).Enqueue(ctx, "welcome:send", map[string]string{
        "user_id": u.ID,
    })
}
```

### Registrace handleru

`provideJobHandlerRegistry` v `infrastructure/di/container_provider.go` — analogie [`providePermissionsRegistry`](/guides/permissions). Worker odmítne enqueue neznámého kindu, neznámý kind v DB → `MarkFailed` (žádný retry).

### Worker flow

1. **Claim** — atomický `UPDATE jobs SET locked_until=…, attempts=attempts+1 … RETURNING *` (skip locked + completed + failed; oldest first).
2. **BeginTx** + injekt dispatcher do ctx (handler může enqueueovat další joby).
3. **handler(txCtx, payload)** — payload je raw JSON bytes; handler si unmarshaluje.
4. Success → `MarkComplete` v stejné tx → `Commit` (atomický business write + completion).
5. Failure / panic → `Rollback` (zahodí handler's DB writes) → `Reschedule` s exponential backoff (`2^(attempts-1) * 5s`, cap 1h), nebo `MarkFailed` po vyčerpání `max_attempts`.

`ServeCommand` spouští worker in-process spolu se schedulerem a serverem; samostatný `./bin/app worker` proces dělá totéž bez HTTP.


## Detaily

- **Mark-complete-in-tx rationale.** Alternativa „mark complete po commitu" otevírá okno crash → duplicate side effect. Náš pattern: handler authors přemýšlejí o idempotenci jen pro *externí* side effects (HTTP, mail), DB writes jsou bezpečně rollbackovatelné.
- **SQLite concurrency.** Default `concurrency=1`. WAL = jeden writer napříč celou DB; víc workerů nezvýší throughput DB-bound handlerů. Bumpnout, jen pokud handlery jsou I/O-bound mimo SQLite.
- **`datetime()` wrap.** Go `time.Time` se ukládá s TZ offsetem (`+02:00`), SQLite `datetime('now')` vrací UTC bez TZ. Claim a další porovnání wrappují obě strany v `datetime(...)` aby SQLite normalizovala. Stejný fix byl aplikován na `token.DeleteExpired` (byl no-op v produkci).
- **No-op dispatcher mimo bus.** `JobDispatcherFromContext` vrací silent dispatcher pokud middleware ctx nepřipravil (CLI bypass, testy bez bus) — handler nikdy nil-checkuje, ale eventy se zahodí.
- **Žádný cascade z event handleru.** Event handler může `Enqueue` (dispatcher je v ctx), ale nesmí volat `EventCollectorFromContext.Collect` (viz [Event Flow](/framework/application/event-flow)).
