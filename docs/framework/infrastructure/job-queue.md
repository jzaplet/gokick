---
layout: 'page'
uri: '/framework/infrastructure/job-queue'
position: 6
slug: 'framework-infrastructure-job-queue'
parent: 'framework-infrastructure'
navTitle: 'Job Queue'
title: 'Job Queue'
description: 'Perzistentní job queue v SQLite — práce, která musí přežít restart. Worker pool, atomický claim, mark-complete-in-handler-tx.'
---

# Job Queue

Perzistentní fronta background práce. Co [Scheduler](/framework/infrastructure/scheduler) je pro periodické maintenance úlohy, Job Queue je pro práci, která **musí přežít restart procesu** — odesílání emailů, externí API volání, retry-prone I/O.


## Proč

In-memory `EventBus` je synchronní — pomalý handler blokuje HTTP response a při SIGTERM se inflight goroutina ztratí. Pro side-effects, které musí proběhnout přesně jednou (best-effort) i po crashi, potřebujeme perzistenci.

Tři tvrdé kontrakty:

1. **Atomicita business write + enqueue.** `CreateUserCommand` zapíše uživatele a enqueueuje welcome job ve stejné transakci. Buď oboje, nebo nic — žádný "uživatel existuje, ale welcome job nebyl naplánován".
2. **At-least-once delivery.** Handler může být zavolán víc než jednou (po crashi, po commit failure). **Handlery musí být idempotentní** pro své externí side-effects.
3. **Mark-complete v handler's transaction.** Když handler zapíše do DB, MarkComplete commitne v té samé tx. Handler failure → tx rollback → handler's DB writes i job completion zmizí společně. Buď je job hotový, nebo se celý retry, žádné částečné stavy.


## Jak

### Komponenty

| Komponenta | Soubor | Role |
|---|---|---|
| `job.Job` entity | `domain/job/job.go` | Persisted job + state (pending / running / completed / failed) |
| `job.Repository` | `domain/job/repository.go` | Doménový port: Enqueue, ClaimDue, MarkComplete, Reschedule, MarkFailed |
| `sqlitejob.Repository` | `infrastructure/sqlite/job/repository.go` | Atomický `UPDATE … RETURNING` claim |
| `shared.JobDispatcher` | `domain/shared/job_dispatcher.go` | Interface pro enqueue z handlerů; ctx-injected |
| `jobapp.Dispatcher` | `application/job/dispatcher.go` | Implementace — JSON marshal payload, validate kind, Enqueue |
| `jobapp.HandlerRegistry` | `application/job/registry.go` | `kind → HandlerFunc` mapa, naplněná v DI |
| `busmw.JobDispatcherMiddleware` | `application/bus/middleware/job_dispatcher.go` | Vloží dispatcher do ctx pro každý CommandBus dispatch |
| `worker.Worker` | `infrastructure/worker/worker.go` | Pool goroutin, claim, run-in-tx, mark-complete, backoff |
| `WorkerCommand` | `presentation/console/worker.go` | `./bin/app worker` — standalone worker proces |

### Stav jobu

State je odvozený z časových sloupců (žádný `status` enum):

| CompletedAt | FailedAt | LockedUntil | Stav |
|:---:|:---:|:---:|---|
| ≠ NULL | — | — | succeeded |
| NULL | ≠ NULL | — | terminálně failed (`max_attempts` vyčerpáno nebo unknown kind) |
| NULL | NULL | ≠ NULL & > now | běží |
| NULL | NULL | NULL nebo < now | pending / retryable |

### Enqueue z command handleru

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... validate, hash password ...
    if err := h.users.Save(ctx, u); err != nil {
        return err
    }

    return shared.JobDispatcherFromContext(ctx).Enqueue(ctx, "welcome:send", map[string]string{
        "user_id": u.ID,
        "email":   u.Email,
    })
}
```

`JobDispatcherFromContext` vrací non-nil dispatcher vždy (mimo bus vrací no-op). `Enqueue` JSON-marshaluje payload, validuje, že kind je registrovaný v `HandlerRegistry`, a volá `repo.Enqueue(ctx, j)`. Insert používá `Conn(ctx)` — když jsme uvnitř `TransactionMiddleware`, lendne do té samé tx jako `Save`. Atomicita garantovaná.

### Worker tok

```
1. ticker (1s default) → processOne(ctx)
2. ClaimDue(ctx, 5min)  — atomicky vybere 1 pending job, bumpne attempts, set locked_until
3. registry.Lookup(kind)
   ├─ neznámý → MarkFailed s "unknown kind" (žádný retry)
   └─ známý → runWithinTx:
       a) BeginTx
       b) Inject dispatcher do txCtx
       c) handler(txCtx, payload)
           ├─ error → Rollback → handleFailure
           ├─ panic → recover → Rollback → handleFailure
           └─ success → MarkComplete v txCtx
       d) Commit (atomic: handler's writes + MarkComplete)
4. handleFailure:
   ├─ attempts >= max_attempts → MarkFailed
   └─ else → Reschedule s exponential backoff (2^(attempts-1) * 5s, capped 1h)
```

### Lifecycle

`ServeCommand` co-runs **scheduler + worker + server**. Jeden ctx z `signal.NotifyContext`, SIGTERM drainuje všechno v tandemu. Standalone `./bin/app worker` spouští jen worker (vhodné pro split deploy: 1× serve replika + N× worker replika).


## Detaily

### Mark-complete-in-handler-tx — proč to není defaultní vzor

Alternativa "marknout complete po handleru" je jednodušší (handler doběhne, commit, pak separate UPDATE), ale otevírá okno: crash mezi handler commitem a mark-complete → job se claimne znovu, side effect aplikovaný 2×. Pro DB-bound jobs to znamená duplicate row.

Naše varianta: MarkComplete je vnitřní krok handler transakce. Buď handler success + MarkComplete + commit jako jedna atomická operace, nebo rollback. Tím je každý handler-retry ekvivalentní s "celá business operace nikdy nestala se" — handler authors přemýšlejí o idempotenci jen pro **externí** side-effects (HTTP API, mailer), ne pro DB writes.

### SQLite concurrency reality

Worker default concurrency = 1 a víc nemá smysl bumpovat: SQLite serializuje writery (WAL: jeden writer napříč celou DB). Pool 4 ≠ 4× throughput pro DB-bound handlery, claim query sama serializuje.

**Kdy bumpnout concurrency:** pokud jsou handlery I/O-bound mimo SQLite (externí HTTP, file I/O), pak víc goroutin pomůže během network blokování. Pro DB-bound práci default 1 stačí. Migrace na Postgres v budoucnu lift-uje tento limit.

### Atomický claim — SQL

```sql
UPDATE jobs
SET attempts = attempts + 1,
    locked_until = datetime('now', '+300 seconds')
WHERE id = (
    SELECT id FROM jobs
    WHERE completed_at IS NULL
      AND failed_at IS NULL
      AND datetime(run_at) <= datetime('now')
      AND (locked_until IS NULL OR datetime(locked_until) < datetime('now'))
    ORDER BY datetime(run_at)
    LIMIT 1
)
RETURNING *
```

Klíčové vlastnosti:

- **UPDATE … RETURNING** v jednom statementu — atomický napříč připojeními.
- `datetime()` wrapper na obou stranách porovnání — Go `time.Time` se serializuje s timezone offsetem (`+02:00`), SQLite `datetime('now')` vrací UTC bez TZ; lex porovnání by selhalo bez normalizace.
- `LIMIT 1` — claim po jednom; batch claim by potřeboval `WHERE id IN (...)` variantu (budoucí optimalizace).
- `ORDER BY datetime(run_at)` — FIFO mezi due joby.

### Exponential backoff

`delay = 2^(attempts-1) * 5s`, capped at 1h. Po claim `attempts` je už incrementovaný, takže:

| attempts | delay |
|---:|---:|
| 1 | 5s |
| 2 | 10s |
| 3 | 20s |
| 4 | 40s |
| 5 | 80s |
| … | … |
| 10+ | 1h (cap) |

### Backward compat: existing F2 fix

`token.TokenRepository.DeleteExpired` měl stejný timezone bug jako moje původní claim SQL (`WHERE expires_at < datetime('now')` neporovnává správně Go RFC3339+TZ proti SQLite UTC). Fix jsem aplikoval v rámci F3 (`WHERE datetime(expires_at) < datetime('now')`) — F2 cleanup job teď skutečně maže expirované tokeny, nejen idle-pings.

### Co tam nepatří

- **Sub-sekundový SLA** — poll interval 1s + claim serialization, real-time práce patří jinam.
- **Workflow / saga orchestrace** — job queue je fire-and-forget. Multi-step workflows (s krokem A → B → C, kompenzacemi) jsou vlastní pattern.
- **Cron-like maintenance** — pro periodické úlohy je [Scheduler](/framework/infrastructure/scheduler) vhodnější (žádná perzistence, žádný overhead).


## Kam dál

| Téma | Odkaz |
|---|---|
| In-process scheduler (cron-like) | [Scheduler](/framework/infrastructure/scheduler) |
| Event flow (DispatchEvents, EventCollector) | [Event Flow](/framework/application/event-flow) |
| Wire DI providers | [Wire DI](/framework/infrastructure/wire) |
| Cobra `serve` / `worker` příkazy | [Console](/framework/presentation/console) |
