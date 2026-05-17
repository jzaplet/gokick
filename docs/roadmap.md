---
layout: 'page'
uri: '/roadmap'
position: 50
slug: 'roadmap'
navTitle: 'Roadmap'
title: 'Roadmap'
description: 'Fázovaný plán dotažení skeletonu do produkce — od opravy event flow přes job queue až po hardening a observabilitu.'
---

# Roadmap

Boilerplate je funkční end-to-end: DDD/CQRS backend, Vue 3 SPA, JWT auth s HttpOnly refresh cookie, admin user CRUD, role-based dashboardy, security headers, perzistovaná session přes hard refresh, produkční Dockerfile + GitHub CI.

Tento dokument popisuje, co zbývá dořešit, aby byl skeleton připravený pro produkci, a kam dál růst nad rámec startovací šablony. Práce je rozdělena do **pěti fází** — první tři dotahují eventy, persistenci a background work (to je kritická cesta pro produkci), čtvrtá přidává hardening, pátá observabilitu.

| Fáze | Téma | Stav |
|---|---|---|
| [1](#fáze-1--stabilizace-event-flow--shutdown) | Stabilizace event flow & graceful shutdown | **Hotovo** |
| [2](#fáze-2--in-process-scheduler) | In-process scheduler (cron-like) | **Hotovo** |
| [3](#fáze-3--perzistentní-job-queue-sqlite) | Perzistentní job queue (SQLite) + worker | **Hotovo** |
| [4](#fáze-4--hardening) | Rate limiting, audit log, brute-force protection | Plánováno |
| [5](#fáze-5--observability) | Sentry, strukturované slog atributy, OpenTelemetry | Plánováno |

Fáze 1–3 řeší **background work** — jejich pořadí je závazné, každá další staví na předchozí (graceful shutdown z F1 je prerekvizita scheduleru z F2, scheduler je prerekvizita worker poolingu v F3). Fáze 4 a 5 jsou nezávislé a lze je řadit podle priority.


## Fáze 1 — Stabilizace event flow & shutdown

**Stav:** Hotovo (2026-05-17).

Než šlo přidat jakýkoli background pattern (scheduler, worker), musel být současný synchronní flow čistý. Fáze opravila **tři defekty** odhalené při analýze:

1. **Race condition v `EventCollector`** — collector byl singleton sdílený mezi paralelními requesty; `Collect`/`Flush` na slice bez locku → eventy se prolévaly mezi commandy.
2. **Pre-commit event dispatch** — `DispatchEventsMiddleware` byla v middleware chainu *uvnitř* `TransactionMiddleware`, takže eventy se dispatchovaly *před* commitem. Selhání commitu znamenalo, že eventy odešly pro data, která nikdy nevzniknou.
3. **Chybějící graceful shutdown** — `http.ListenAndServe` bez signal handlingu, SIGTERM zabíjel proces uprostřed inflight requestu.

### Co bylo uděláno

- [x] **Request-scoped `EventCollector`**
  - `domain/shared/event.go`: přidány `ContextWithEventCollector(ctx)` + `EventCollectorFromContext(ctx)`; `EventCollector` má teď `sync.Mutex` pro případ goroutin uvnitř handleru. `EventCollectorFromContext` mimo bus vrací throwaway collector (vhodné pro CLI bypass).
  - `application/bus/middleware/events.go`: `DispatchEventsMiddleware` vytváří collector v `ctx`, po `next()` flushne. Konstruktor už nedostává `*EventCollector`.
  - `application/user/command/create_user.go`: `events` field odstraněn, handler volá `shared.EventCollectorFromContext(ctx).Collect(...)`.
  - `infrastructure/di/container_provider.go`: smazán `provideEventCollector`, signatura `provideCommandBus` zúžena.
  - `internal/testfx/testfx.go`: `NewBuses()` vrací jen `(*CommandBus, *QueryBus, *EventBus)`.

- [x] **Middleware reorder: DispatchEvents OUT of Transaction**
  - V `provideCommandBus`: `Recovery → Logging → Authorize → DispatchEvents → Transaction → handler`.
  - Při neúspěšném commitu chyba propaguje skrz `DispatchEvents`, flush se přeskočí.

- [x] **Graceful shutdown HTTP serveru**
  - `cmd/main.go`: `signal.NotifyContext(ctx, SIGINT, SIGTERM)`.
  - `app/application.go`: `Run(ctx) error`.
  - `presentation/console/root.go`: `Execute(ctx) error` → `ExecuteContext(ctx)`.
  - `presentation/console/serve.go`: `cmd.Context()` propaguje do `server.Start`.
  - `presentation/http/server/server.go`: `Start(ctx) error` — `http.Server` + `Shutdown(shutdownCtx)` s 30s timeoutem; `ListenAndServe` v goroutině, hlavní `select` čeká na chybu nebo `ctx.Done()`.

- [x] **Dokumentace sjednocena s realitou**
  - `framework/overview/architecture.md`, `framework/application/bus.md`, `framework/application/events.md`, `framework/application/commands.md`, `framework/domain/errors-events.md`, `framework/presentation/console.md`, `framework/presentation/http-server.md`, `framework/infrastructure/wire.md` — opraveny zmínky o singletonu collectoru, pořadí middleware, sync/async dispatch a startup sekvenci.

### Regresní testy

- `app/application/bus/middleware/events_test.go::TestDispatchEventsMiddleware_PerRequestIsolation` — 200 paralelních dispatchů přes `CommandBus`, ověřuje, že každý dispatch dostane unikátní event přesně 1× (žádné cross-contamination). Chrání proti regresi zpět na singleton collector.
- `app/application/bus/middleware/events_test.go::TestEventCollector_Collect_ConcurrentWriters` — 50 goroutin × 100 `Collect` proti jednomu collectoru ověřuje mutex.
- `app/presentation/http/server/server_test.go::TestGracefulShutdown_DrainsInflightRequest` — handler blokuje na release channelu, `Shutdown` je volán mid-flight a test ověřuje, že request nevrátí 200 dřív než handler doběhne.

### Definition of Done — splněno

- ✅ `go test -race ./app/... ./cmd/...` projde čistě (včetně nových regresních testů).
- ✅ `make arch-check` projde.
- ✅ `golangci-lint` 0 issues.
- ✅ `CLAUDE.md` aktualizován (middleware order + per-request collector pattern).
- ✅ Ověřeno manuálně: `kill -TERM $PID` na běžícím serveru → log `server: shutdown signal received, draining` → `server: stopped`, exit code 0.


## Fáze 2 — In-process scheduler

**Stav:** Hotovo (2026-05-17).

Cron-like spouštění periodických úkolů uvnitř `serve` procesu. Žádný externí cron, žádný DB-backed scheduler — goroutiny s tickerem registrované přes Wire DI. První konkrétní uživatel: cleanup expirovaných `refresh_tokens`.

### Co bylo uděláno

- [x] **`infrastructure/scheduler/scheduler.go`**
  - API: `NewScheduler(logger, []Job{...})` + `Run(ctx)`. Constructor validuje unikátnost jmen, nenulové intervaly, non-nil Fn.
  - Per-job goroutina s `time.Ticker` + `select` na `ctx.Done()`.
  - **Run-once-then-tick** semantika: Fn proběhne ihned po startu, pak periodicky. Garantuje aspoň jeden cleanup za lifetime frekventně restartovaného procesu.
  - **Panic recovery per-tick**: panicující job se zaloguje a další tick proběhne normálně; sourozenecké joby nejsou ovlivněny.
  - Error z Fn se loguje, ale tikání pokračuje (idempotentní semantika).

- [x] **Lifecycle v `ServeCommand`**
  - `RunE` spustí `scheduler.Run(ctx)` v goroutině před `server.Start(ctx)`.
  - Společný `ctx` z `signal.NotifyContext` → SIGTERM drainuje scheduler i server v tandemu.
  - `schedulerDone` channel garantuje, že `RunE` nevrátí, dokud scheduler nedrainuje.

- [x] **Refresh token cleanup job**
  - `Name: "cleanup:expired-refresh-tokens"`, `Interval: 1h`, `Fn: tokens.DeleteExpired`.
  - `DeleteExpired` ponechán beze změny — `WHERE expires_at < datetime('now')`. Rozšíření o `used_at` bylo původně v roadmapě, ale po review zahozeno (used token zůstává v DB do `expires_at` pro theft-detection okno; smazat dřív = ztráta signálu bez bezpečnostního přínosu).

- [x] **`.go-arch-lint.yml`** — přidána komponenta `scheduler` (`infrastructure/scheduler/**`), rozšířen `console.mayDependOn` o `scheduler`.

- [x] **DI** — `provideScheduler(logger, tokens) (*scheduler.Scheduler, error)` v `container_provider.go`. Wire propojí `*Scheduler` → `ServeCommand`. Validation error z constructoru bublí přes `CreateApplication` → fail-fast při startu.

### Regresní testy

`app/infrastructure/scheduler/scheduler_test.go` (7 testů):

- `TestScheduler_RunsAndStops` — krátký interval (10ms) + counter; ověří run-once-then-tick + graceful drain
- `TestNewScheduler_DuplicateName` — duplicitní jméno → error z constructoru
- `TestNewScheduler_InvalidJob` — prázdné jméno / nulový interval / nil Fn (3 subtests)
- `TestScheduler_PanicInOneJobKeepsOthersRunning` — panic v jednom jobu, sourozenecké pokračují
- `TestScheduler_ErrorReturnedJobKeepsTicking` — error z Fn nezhasí ticker
- `TestScheduler_ImmediateCancelDoesNotHang` — cancel mezi ticky preempuje ticker

### Definition of Done — splněno

- ✅ `go test -race ./app/...` všech 7 nových testů projde.
- ✅ `make arch-check` projde s novou `scheduler` komponentou.
- ✅ `golangci-lint` 0 issues.
- ✅ Manuální smoke: `make serve` → log `scheduler: starting jobs=1` + `cleanup:expired-refresh-tokens` proběhl 333µs po startu (run-once tick) + `SIGTERM` → `scheduler: stopped` před `server: stopped`, exit 0.


## Fáze 3 — Perzistentní job queue (SQLite)

**Stav:** Hotovo (2026-05-17).

Práce, která **musí přežít restart procesu nebo crash**: odesílání emailů, externí API volání, cokoli I/O-heavy nebo retry-prone. In-memory `EventBus` na to není stavěný — synchronní dispatch zablokuje response, async goroutina se ztratí při SIGTERM.

### Klíčová rozhodnutí

| Otázka | Volba |
|---|---|
| **Jak worker volá handler?** | Worker má vlastní `runWithinTx` — ne celý middleware chain, jen `BeginTx → handler → MarkComplete → Commit` (rollback při error). Jednodušší než CQRS bus, plus mark-complete-in-handler-tx semantika. |
| **Mark-complete kdy?** | **Uvnitř handler transakce** (advisor rec). Handler write + MarkComplete commitují atomicky. Handler-fail = celá tx rollback (včetně handler's DB writes) → re-claimable. Idempotence se týká jen *externích* side-effects. |
| **Delivery semantika** | At-least-once. Handlery **musí být idempotentní** pro externí side effects (mail, API). |
| **Failure handling** | Exponenciální backoff `2^(attempts-1) * 5s`, cap 1h. `max_retries` exhausted → `failed_at` + `last_error`. |
| **Concurrency default** | **1 worker.** SQLite serializuje writery (WAL: one writer at a time) — víc goroutin nezvýší throughput pro DB-bound joby. Bumpnout, jen pokud handlery jsou I/O-bound mimo SQLite. |
| **JobDispatcher** | Context-injected (analogie `EventCollectorFromContext`), ne přes konstruktor. Bus middleware vkládá dispatcher do ctx; handler volá `shared.JobDispatcherFromContext(ctx).Enqueue(...)`. |
| **Atomický claim** | `UPDATE jobs SET locked_until=... WHERE id=(SELECT id FROM jobs WHERE due AND not_locked LIMIT 1) RETURNING *`. Wrap obou stran porovnání `datetime()` — Go time.Time má TZ offset, SQLite `datetime('now')` je UTC bez TZ; lex porovnání by selhalo. |

### Co bylo uděláno

- [x] **Migrace** — `20260517000001_create_jobs_table.sql` (id, kind, payload, run_at, attempts, max_retries, locked_until, last_error, failed_at, completed_at, created_at + partial index pro claim).
- [x] **`domain/job/`** — `Job` entity, `Repository` interface (Enqueue, ClaimDue, MarkComplete, Reschedule, MarkFailed, FindByID).
- [x] **`domain/shared/job_dispatcher.go`** — `JobDispatcher` interface (povinný `maxRetries` poziční parametr) + `ContextWith/FromContext` helpers + `WithDelay` option + no-op fallback dispatcher.
- [x] **`infrastructure/sqlite/job/`** — atomický claim přes `UPDATE … RETURNING`.
- [x] **`application/job/`** — `Dispatcher` (JSON marshal, kind validation), `HandlerRegistry` (constructor-time empty kind check, immutable lookup).
- [x] **`application/bus/middleware/job_dispatcher.go`** — vkládá dispatcher do ctx před TransactionMiddleware, takže `Enqueue` v handleru se připojí do business tx.
- [x] **`infrastructure/worker/worker.go`** — pool goroutin, claim, **runWithinTx (BeginTx → handler → MarkComplete → Commit)**, panic recovery, exponential backoff, ctx-driven drain.
- [x] **`presentation/console/worker.go`** — `./bin/app worker` standalone příkaz; `ServeCommand` zároveň co-runs in-process worker s scheduler+server (sdílí jeden ctx).
- [x] **`.go-arch-lint.yml`** — nová `worker` komponenta, rozšířen `console.mayDependOn` o `worker`, `sqlite_repos`/`worker` mohou importovat `testfx`.
- [x] **Bonus fix:** `token.TokenRepository.DeleteExpired` měl stejný TZ-format bug jako moje původní claim (no-op v praxi). Opraveno v rámci F3 — F2 cleanup teď reálně maže expired tokeny.

### Regresní testy

- `app/infrastructure/sqlite/job/repository_test.go` (8 testů) — Enqueue/FindByID, ClaimDue empty/skipsLocked/picksOldest/**atomicConcurrent** (20 jobs × 40 goroutines, každý job claimnut přesně 1×), MarkComplete/Reschedule/MarkFailed s lifecycle ověřením.
- `app/infrastructure/worker/worker_test.go` — handler success/failure/panic, **mark-complete-in-tx atomicity** (handler write + completion commit atomicky; handler-fail rollbackne i handler's writes), retries respect maxRetries boundary, unknown kind no-retry, cascade Collect panics.
- `app/application/job/dispatcher_test.go` (3 testy) — Enqueue valid kind round-trip + JSON payload, unknown kind → error, empty kind v registry → error.

### Definition of Done — splněno

- ✅ `go test -race ./app/... ./cmd/...` všech 17 nových testů projde.
- ✅ `make arch-check` projde s novou `worker` komponentou.
- ✅ `golangci-lint` 0 issues.
- ✅ Manuální smoke: `make serve` → log `worker: starting concurrency=1 kinds=[]` → SIGTERM → `scheduler:`, `worker:`, `server: stopped` v správném pořadí, exit 0.


## Fáze 4 — Hardening

Nezávislé bezpečnostní zpevnění. Lze řadit/odsunout podle priority projektu.

### Úkoly

- [ ] **Rate limiting na auth endpointech**
  - Token bucket nebo sliding window per IP (extrahované z `X-Forwarded-For` při běhu za reverse proxy).
  - Primárně `POST /api/v1/auth/login` a `POST /api/v1/auth/refresh`.
  - Implementace: middleware v `presentation/http/middleware/ratelimit.go`, in-memory (sync.Map + bucket struct) — pro single-binary dostačuje. Při multi-replica setupu pozdější přechod na Redis nebo SQLite-backed counter.
  - Konfigurace: `APP_RATE_LIMIT_LOGIN=10/min`, `APP_RATE_LIMIT_REFRESH=60/min`.

- [ ] **Audit log**
  - Nová tabulka `audit_log` (`id`, `actor_user_id`, `actor_ip`, `action`, `target_type`, `target_id`, `metadata` JSON, `created_at`).
  - Append-only — žádné `UPDATE`/`DELETE`.
  - Sběr v command handlerech přes nový `domain/shared/audit.go AuditLogger` interface (analogie `EventCollector`, ale flush mimo transakci, aby selhání auditu neshodilo command).
  - Příklady událostí: login success/failure, role change, user delete, password change, refresh token theft detection.
  - Užitečné jak pro compliance, tak pro debugging incidentů.

- [ ] **Brute-force ochrana přes account lock**
  - Doplněk k rate limitingu — 5 failed login attempts za 10 min ⇒ account lock na 15 min.
  - Sloupec `users.locked_until` (`DATETIME NULL`), počítadlo `failed_login_attempts` (INTEGER) reset na úspěšný login.
  - `LoginHandler` po failed pokusu inkrementuje, po locked-out vrací neutrální chybu (neprozrazuje, že je účet zamčený).

- [ ] **CSRF token endpoint (volitelně)**
  - Dnes `http.CrossOriginProtection` (Go 1.25 stdlib) řeší same-site případ.
  - Až by SPA běžela na jiném originu, doplnit double-submit cookie pattern: endpoint `GET /api/v1/csrf` vrací token, frontend ho posílá v `X-CSRF-Token` hlavičce, server porovnává s cookie hodnotou.


## Fáze 5 — Observability

Až aplikace začne jezdit v produkci. Bez F1–F3 by observabilita měřila nestabilní systém.

### Úkoly

- [ ] **Strukturované slog atributy — audit konzistence**
  - Sjednotit naming napříč middleware vrstvami: `trace_id`, `user_id`, `command`, `duration_ms`, `event`, `job_kind`.
  - Doplnit `user_id` do bus `LoggingMiddleware` (dnes loguje jen `trace_id` a `command`).
  - Vytvořit helper `shared.LogAttrs(ctx) []slog.Attr` — vrací standardní set atributů z contextu.

- [ ] **Sentry**
  - `APP_SENTRY_DSN` env. Když je prázdné, Sentry se neinicializuje.
  - Recovery middleware (HTTP i bus) hlásí panic s `trace_id`, `user_id`, `command/path` v contextu.
  - Worker stejně — failed job po exhausted `max_retries` jde do Sentry s `kind`, `payload` (truncated), `last_error`.

- [ ] **OpenTelemetry (volitelně, později)**
  - Až bude nasazená alespoň jedna další služba (database proxy, search backend, atd.). Pro standalone monolit přidává komplexitu bez návratnosti.
  - OTel HTTP middleware + propagace přes bus middleware. `traceID` v contextu může přejít na `trace.SpanContext`.
  - Pro tracing job workeru: span per job s `kind` a `attempts` jako atributy.


## Co je už hotové

Pro úplnost — tyto věci nejsou v žádné fázi, protože jsou stabilní a produkčně použitelné:

- **Auth flow:** login (cookie + access token), silent refresh při bootu, 401 auto-retry s single-flight refresh, theft detection přes `used_at` marker.
- **Admin user CRUD:** list / create / update / delete s field-keyed validation errors, self-delete protection, role change na vlastním účtu vyvolá full-page reload kvůli refresh JWT.
- **Build & deploy:** 3-stage produkční Dockerfile (Vite SPA → Go binary → Alpine runtime), `docker-compose.yml` s healthcheck, `.github/workflows/validate.yml` (install → lint → test → build), Documan auto-start přes `make documan-*`.
- **Migrace:** konsolidovaná do jediné `init_schema.sql` — fresh deploy je čistý. Nové migrace mít vyšší timestamp.
- **CSRF:** `http.CrossOriginProtection` (Go 1.25 stdlib) pro same-site případ.
