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
| [4](#fáze-4--hardening) | Rate limiting, audit log, brute-force protection | **Hotovo** |
| [5](#fáze-5--observability) | Sentry, strukturované slog atributy, OpenTelemetry | Probíhá |

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

**Stav:** Hotovo (2026-05-17).

Před přidáním nových funkcí proběhl důkladný bezpečnostní audit, který odhalil tři exploitovatelné vady a sedm hardening položek nad rámec původně plánovaných tří F4 témat. Vše do jednoho PR rozděleného na šest commitů (audit fix → critical → hardening → rate limit → brute-force → audit log → polish).

### Klíčová rozhodnutí

| Otázka | Volba |
|---|---|
| **XFF trust pro rate limit** | Žádný XFF parsing. Default `RemoteAddr`; `APP_TRUST_PROXY_HEADERS=true` opt-in čte `X-Real-IP`. XFF + spoof bypass je footgun, ne feature. |
| **Audit middleware umístění** | Outside `Transaction` i `DispatchEvents`. Flushuje **i na error** (login_failed musí persistovat při AuthError). Audit-write failure se loguje, ale nepropaguje. |
| **Brute-force vs login timing** | Vždy `Verify` (proti dummy hashi pokud user nebo lock) → pak větvení. Lock check **po** Verify, jinak by čas odpovědi prozradil lock state. |
| **Atomic failed-login counter** | Single `UPDATE` s vnořeným `CASE` — žádný read-modify-write race. Counter reset na 0 po locku; reset window 10 min. |
| **Lock policy** | 5 failed/10min ⇒ lock 15min. Útoky na locked účet nezvyšují counter (no-op + audit). |
| **CSRF token endpoint** | Vynecháno dle dohody — `http.CrossOriginProtection` (Go 1.25) stačí pro same-site. |

### Co bylo uděláno

- [x] **Kritické fixy z auditu**
  - **XSS přes `ToastContainer.vue v-html`** — odstranění `v-html`, render přes `{{ }}`.
  - **Refresh-token race** — `MarkUsed` nově `UPDATE ... WHERE token_hash=? AND used_at IS NULL` + `RowsAffected==1` guard; loser → theft detection.
  - **Default admin seed pwd** — vyžaduje `APP_SEED_ADMIN_PASSWORD` validovaný přes `user.NewPassword`; seeder přesunut do `infrastructure/sqlite/seeder/`.

- [x] **HTTP boundary hardening**
  - `http.Server` timeouty (ReadHeader/Read/Write/Idle) + `MaxHeaderBytes 64 KiB`.
  - Nový `presentation/http/request` package: `DecodeJSON` aplikuje `MaxBytesReader 1 MiB` + `DisallowUnknownFields`; všechny handlery přepnuty.
  - `response.HandleError` vrací generic `"internal server error"` pro non-HTTPError (žádný leak DB/panic stringů).
  - `TraceMiddleware` validuje inbound `X-Trace-Id` (regex `[A-Za-z0-9_-]{8,64}`) — blokuje log injection + spoofing.
  - `LoginHandler` precomputuje dummy bcrypt hash při startu a vždy `Verify` (nelze časem zjistit existenci uživatele ani lock state).
  - `NewJwtService` odmítá `APP_JWT_SECRET` kratší než 32 znaků (RFC 7518).
  - `UpdateUserHandler` blokuje self-demote z admin role (mirror existující `DeleteUser` self-lockout guardu).

- [x] **Rate limiting**
  - `presentation/http/middleware/ratelimit.go`: per-IP token bucket, background janitor drop idle ≥ 5min.
  - Defaults `APP_RATE_LIMIT_LOGIN=10/min`, `APP_RATE_LIMIT_REFRESH=60/min`. Parse: `N/sec|min|hour|Xs|Xm|Xh`; empty = disabled.
  - `IPExtractor` sdílen s audit IP middleware — flip `APP_TRUST_PROXY_HEADERS` mění oba.

- [x] **Brute-force account lock**
  - Migration `20260517000002_add_user_lock_columns.sql` (`failed_login_attempts INTEGER`, `last_failed_login_at DATETIME`, `locked_until DATETIME`).
  - `user.Repository.RecordFailedLogin` / `ResetFailedLogin` — single atomic SQL UPDATE, **outside** caller's tx (counter musí přežít AuthError rollback).
  - `LoginHandler`: po Verify větví. Lock check po Verify → konstantní čas. Locked attempty = no-op + audit `auth.login.blocked_while_locked`.

- [x] **Audit log**
  - Migration `20260517000003_create_audit_log.sql` (append-only `audit_log` s `actor_user_id`, `actor_ip`, `action`, `target_*`, JSON `metadata`).
  - `domain/shared/audit.go`: `AuditEvent`, `AuditCollector` (mutex-safe), `AuditLogger` port, `ContextWithActorIP` helpers.
  - `application/bus/middleware/audit.go`: wraps outside Transaction; flushuje regardless of err přes `context.WithoutCancel`. Write failure se loguje, ne propaguje.
  - `infrastructure/sqlite/audit/repository.go`: `r.DB.DB()` (raw pool, mimo business tx).
  - `presentation/http/middleware/ip.go`: stash IP do ctx pro audit.
  - Handler integrace: `auth.login.{succeeded,failed,blocked_while_locked}`, `auth.account.locked`, `auth.token.theft_detected` (2 reasons), `user.{created,role_changed,deleted}`, `user.password_changed`.

- [x] **Polish**
  - CORS přidá `Vary: Origin` (shared cache safety).
  - `SqliteManager` whitelistuje `APP_DB_JOURNAL_MODE` na `WAL|DELETE|MEMORY`.
  - `clearRefreshCookie` kombinuje `MaxAge=-1` + `Expires=epoch` (legacy fallback).

- [x] **SQLite & concurrency hardening (late phase-4)**
  - **Deadlock fix `CreateUser` & spol.** — DSN `SqliteManager` přešel z holé cesty na `file:<path>?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)`. Bus tx pattern read → bcrypt (200 ms) → write pod default DEFERRED tx ztrácel read snapshot, jakmile mezitím commitnul worker/scheduler poll, a follow-up zápis fail-fast jako `SQLITE_BUSY_SNAPSHOT` ("database is locked", busy_timeout to nepokrývá). IMMEDIATE bere write lock při BEGIN → snapshot zůstane validní. `foreign_keys` se nově aplikuje na všechny pool konexe (per-conn pragma).
  - **`ClaimDue` flake root fix** — F3 přešlo z `datetime()` na `strftime('%f', ...)` pro sub-sekundovou precizi. Probe ale odhalil dvě hlubší trhliny: `strftime('%f', t)` zaokrouhluje na ms round-half-up (Go time s µs ≥ 500 končí o ms napřed) a ncruces WASM `'now'` trailí Go `time.Now()` o ~1 ms. Oprava: srovnání přepsáno na `julianday(...)`, `Enqueue`/`Reschedule` v repo truncate `run_at` na `UTC + ms` přes `msPrecisionUTC` -- obě strany srovnání mají společnou precizi.
  - **`login_test` arch-lint root fix** — `TestLoginHandler_DoesNotDeadlockUnderCommandBus` importoval `application/bus` přímo (porušení pravidla "application může záviset jen na bus_middleware, ne na bus samotném"). Místo excludeFile workaround přidán `testfx.ExecCommand[R]` wrapper kolem `bus.Exec` (testfx je sanctioned escape hatch); test přepojen.
  - Regresní testy: `app/infrastructure/database/sqlite_manager_test.go` (`TestSqliteManager_ConcurrentTxWritesDoNotReturnBusy`, 4 goroutines × 25 iterací read-hold-write v tx -- bez fixu padá hned na first iteration); `app/infrastructure/sqlite/job/repository_test.go` stabilní 100× v řadě.

### Regresní testy

- `app/presentation/http/request/decode_test.go` — body size, unknown fields, trailing JSON
- `app/presentation/http/response/response_test.go` — generic 500 sanitization
- `app/presentation/http/middleware/trace_test.go` — log injection rejection
- `app/presentation/http/middleware/ratelimit_test.go` — parse, bucket allow/refill, sweep, 429+Retry-After, IP extractor matrix
- `app/infrastructure/sqlite/user/repository_test.go` — RecordFailedLogin (increment, threshold, window), ResetFailedLogin
- `app/infrastructure/sqlite/token/repository_test.go` — `MarkUsed` race guard
- `app/infrastructure/sqlite/audit/repository_test.go` — Save persistence
- `app/infrastructure/sqlite/seeder/seeder_test.go` — required password validation
- `app/domain/shared/audit_test.go` — collector concurrency, throwaway fallback
- `app/application/bus/middleware/audit_test.go` — flush on err, actor/IP stamping, persist failure swallowed
- `app/application/auth/command/login_test.go` — brute-force lock + audit recording (login.succeeded, login.failed, account.locked)
- `app/application/user/command/update_user_test.go` — self-demote blocked

### Definition of Done — splněno

- ✅ `go test -race ./app/... ./cmd/...` všech nových + existujících testů projde
- ✅ `make arch-check` projde s novými komponentami `sqlite_seeder` (`infrastructure/sqlite/seeder/**`) a `request` (`presentation/http/request/**`). Audit repo **nemá** vlastní komponentu — `infrastructure/sqlite/audit/**` je položkou enumerovaného `sqlite_repos` (vedle `job`/`token`/`user`).
- ✅ Manuální smoke: `make serve` + curl proti `/auth/login` s 11 wrong passwords → 11. 429 + Retry-After; po 10 valid attempts → po 5 failed `auth.account.locked` row v `audit_log`.


## Fáze 5 — Observability

**Stav:** Probíhá — strukturované slog atributy + Sentry (BE i FE) hotové (2026-06-10), produkční hardening + obohacení eventu (2026-06-14); zbývá už jen OpenTelemetry (volitelně).

Až aplikace začne jezdit v produkci. Bez F1–F3 by observabilita měřila nestabilní systém.

### Úkoly

- [x] **Strukturované slog atributy — audit konzistence** — Hotovo (2026-06-10).
  - `app/domain/shared/log.go`: konstanty klíčů (`LogKeyTraceID/UserID/Command/DurationMs/RetryInMs/Error/Event/JobKind`), `LogAttrs(ctx) []slog.Attr` (jediný zdroj korelace `trace_id` + `user_id`, zároveň šev pro budoucí `span_id`), `DurationMsAttr`/`MillisAttr` (číselné `duration_ms` ve zlomku ms, µs přesnost).
  - `user_id` doplněn do bus `LoggingMiddleware`; korelace přes `LogAttrs` i v recovery / events / audit middleware a HTTP request logu.
  - Sjednoceno napříč vrstvami: `duration`→`duration_ms` (bus / HTTP / worker / scheduler), worker `kind`→`job_kind`, `attempt`→`attempts`, `retry_in`→`retry_in_ms`. Komponentně-lokální klíče (`addr`, `slot`, `name`, `nickname`) ponechány.
  - Logger constructor vyextrahován do `cmd/logger.go` (testovatelný, env-driven přes `APP_LOG_FORMAT` / `APP_LOG_LEVEL`) — záměrně jediný šev, kam později zapadne OTel handler. Viz [Observability](/framework/infrastructure/observability).
  - **Statické vynucení** (`.golangci.yml`): `depguard` (zákaz cizích loggerů) + `forbidigo` (`fmt.Print*`, stdlib `log`, `slog.New*` mimo `cmd/`, `os.Stdout/Stderr`) + `sloglint` (`no-global`, `static-msg`, `no-raw-keys`, `key-naming-case: snake`, `no-mixed-args`). Tím nelze logovat jinou cestou — všechny klíče převedeny na konstanty (cross-cutting `shared.LogKey*`, komponentní `logKey*`). Ověřeno probem se všemi bypass vektory.
  - Testy: `app/domain/shared/log_test.go`, `app/application/bus/middleware/logging_test.go`, `cmd/logger_test.go`.

- [x] **Sentry (BE + FE)** — Hotovo (2026-06-10). Rozsah A: jen chyby & paniky.
  - Port `shared.ErrorReporter` (`Capture`/`Flush`) + `NopReporter`; staven v `cmd/sentry.go`, gated na `APP_SENTRY_DSN` (prázdné = no-op). `defer Flush` v `main` (+ před `os.Exit`), protože `CaptureException` je async.
  - **Přidán HTTP `RecoveryMiddleware`** (dosud chyběl) — panika mimo bus → log + report + 500. Bus recovery + worker (exhausted retries) hlásí taky.
  - **FE:** `@sentry/vue` v `assets/app.ts`; Vue chyby + unhandled rejections. Follow-up: source-map upload (`@sentry/vite-plugin`) pro čitelné traces.
  - sentry-go vědomě přidán do depguard allowlistu (jinak ho enforcement blokuje). Viz [Observability](/framework/infrastructure/observability).

- [x] **Sentry — produkční hardening + obohacení** — Hotovo (2026-06-14, E2E test šablony přes reálnou prod pipeline; [gokick PR #11](https://github.com/jzaplet/gokick/pull/11)). Ověřeno na živém deploy.
  - **FE Sentry v prod** — `VITE_SENTRY_DSN` je build-time, takže prod image (buildnutý jednou) ho neměl → FE Sentry byl tmavý. Nově server injektuje FE config (`APP_SENTRY_DSN_FRONTEND`, environment, debug) do `index.html` jako `<meta>` tagy, SPA čte runtime (`runtimeConfig.ts`); CSP `connect-src` se otevře na ingest origin.
  - **Reálná klientská IP** — `IPExtractor` čte `CF-Connecting-IP` (→ `X-Real-IP` → `RemoteAddr`); IP v access logu i na panic logu. Dokumentován **Cloudflare origin-lock** ([Config](/framework/infrastructure/config#app_trust_proxy_headers--cloudflare-origin-lock)).
  - **Obohacení eventu** — User (id/nickname/role + IP, `user.ip_address`) + Request (method/url/User-Agent z whitelistu, nikdy syrové hlavičky); `SendDefaultPII:true`. Access log + `status`/`bytes`. Klíče `method`/`path`/`url`/`user_agent` povýšeny do `shared.LogKey*`.
  - **Oprava pořadí middleware** — `Trace → IP → Recovery` (IP před Recovery), aby HTTP-recovery capture nesl klientskou IP. Regresní test proti `buildMiddlewareChain`.
  - **`APP_SENTRY_DEBUG`** — gated BE `/debug/sentry` panika + FE tlačítko pro smoke-test. Návod: [Sentry guide](/guides/sentry).
  - **Kvalita eventu (2. kolo)** — paniky mají typ `panic` (místo `*errors.errorString`) + culprit na reálném místě (`in_app` degradace reporting framů) přes `shared.PanicError`; BE eventy nesou **breadcrumbs** (per-request hub + `breadcrumbHandler` nad slog → trail `INFO+` logů, jako Symfony); FE **source maps** přes `@sentry/vite-plugin` (upload + smazání z dist, debug-ID, opt-in `SENTRY_AUTH_TOKEN`). Vše ověřeno BeforeSend integračními testy.

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
