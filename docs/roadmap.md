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
| [2](#fáze-2--in-process-scheduler) | In-process scheduler (cron-like) | Plánováno |
| [3](#fáze-3--perzistentní-job-queue-sqlite) | Perzistentní job queue (SQLite) + worker | Plánováno |
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

Cron-like spouštění periodických úkolů uvnitř `serve` procesu. Žádný externí cron, žádný DB-backed scheduler — jen goroutina s tickerem registrovaná z DI. První konkrétní uživatel: cleanup expirovaných `refresh_tokens` (`token.TokenRepository.DeleteExpired` už v doméně existuje, jen ho nikdo nevolá).

### Úkoly

- [ ] **`infrastructure/scheduler/scheduler.go`**
  - API: `Every(interval time.Duration, name string, fn func(ctx context.Context) error)`.
  - Graceful shutdown přes `ctx.Done()` — propojí se s `ctx` z Fáze 1.
  - Per-job logging (`name`, `duration`, `error`) přes injektovaný `*slog.Logger`.
  - Při startu validuje, že žádné dva joby nemají stejný `name`.

- [ ] **Spuštění ze serveru**
  - V `serve` příkazu (nebo přes `Application.Run`) vytvořit scheduler, registrovat joby a spustit ve své goroutině před `server.Start`.
  - Při shutdown signal: nejprve scheduler stop, pak HTTP server stop.

- [ ] **Refresh token cleanup job**
  - `scheduler.Every(1*time.Hour, "cleanup:expired-refresh-tokens", tokens.DeleteExpired)`.
  - Tip: rozšířit kritérium v `DeleteExpired` o `used_at < now() - theftWindow` (např. 24h), aby se mazaly i už rotované tokeny po dohrání theft-detection okna.
  - Dispatch jde mimo `CommandBus` (žádný auth context, žádná transakce nutná — `DELETE` v jedné statement je atomický). Job dostává čistý `context.WithTimeout` z scheduleru.

- [ ] **`.go-arch-lint.yml`** — přidat komponentu `scheduler` (a později `worker` ve Fázi 3). Wildcard `infrastructure/**` neexistuje, každá nová podsložka potřebuje explicitní záznam.

### Definition of Done
- `make serve`, počkat hodinu, `refresh_tokens` se vyčistí, log obsahuje řádek `scheduler: job completed name=cleanup:expired-refresh-tokens duration=…`.
- `SIGTERM` ukončí scheduler před HTTP serverem; žádný job se neukončí uprostřed.


## Fáze 3 — Perzistentní job queue (SQLite)

Práce, která **musí přežít restart procesu nebo crash**: odesílání emailů, externí API volání, cokoli I/O-heavy nebo retry-prone. In-memory `EventBus` na to není stavěný — synchronní dispatch zablokuje response, async goroutina se ztratí při SIGTERM.

### Návrhová rozhodnutí (před implementací)

| Otázka | Doporučení |
|---|---|
| **Jak worker volá handler?** | Vlastní zkrácený middleware chain: `Recovery → Logging → Transaction`. **Bez** `Authorize` (job nemá auth claims) a **bez** `DispatchEvents` (job sám může collectovat eventy, ale ne kaskádovat). |
| **Delivery semantika** | At-least-once. Handlery **musí být idempotentní** — kontrakt v dokumentaci. |
| **Failure handling** | Exponenciální backoff (`run_at = now + 2^attempts * baseDelay`), `max_attempts` na typ jobu, po vyčerpání → status `failed` + `last_error`. |
| **Concurrency** | `SELECT … FOR UPDATE SKIP LOCKED` SQLite nepodporuje. Atomický claim přes `UPDATE jobs SET locked_until=... WHERE id=(SELECT id FROM jobs WHERE run_at<=now() AND (locked_until IS NULL OR locked_until<now()) LIMIT 1) RETURNING *` v jedné transakci. |
| **Event → job bridge** | Opt-in: event handler dostane `JobDispatcher` v konstruktoru a sám rozhodne, jestli běží sync, nebo enqueue. Default zůstává sync — heavy handlery se explicitně přepnou. |

### Úkoly

- [ ] **Migrace** — nová `migrations/2026xxxxxxxxxx_create_jobs_table.sql`:
  - sloupce: `id` (UUIDv7 TEXT PK), `kind` (TEXT), `payload` (TEXT, JSON), `run_at` (DATETIME), `attempts` (INTEGER), `max_attempts` (INTEGER), `locked_until` (DATETIME NULL), `last_error` (TEXT NULL), `created_at`, `completed_at` (NULL).
  - indexy: `(run_at, locked_until)` pro claim query, `(kind, completed_at)` pro monitoring.

- [ ] **`domain/job/`** — nový bounded kontext.
  - `Job` entity, `Kind` value object (whitelist přes registry, jinak `ValidationError`), `Repository` interface: `Enqueue`, `ClaimDue(ctx, limit, lockFor)`, `Complete(ctx, id)`, `Fail(ctx, id, err, retryIn)`.
  - Žádný cross-kontext import — pokud event handler chce enqueueovat, dostane `application.JobDispatcher` interface, ne `*job.Repository`.

- [ ] **`infrastructure/sqlite/job/`** — implementace, claim přes `UPDATE … RETURNING *`.

- [ ] **`application/job/`**
  - `JobDispatcher` interface (handlery na něj depend místo na repo).
  - `JobHandlerRegistry` — mapuje `kind` → `func(ctx context.Context, payload []byte) error`. Naplněno z Wire (každý handler se registruje v `container_provider.go` jak u permission registry).

- [ ] **`infrastructure/worker/worker.go`**
  - Goroutine pool (konfigurovatelná concurrency přes `APP_WORKER_CONCURRENCY`, default 4).
  - Poll interval (default 1s) + `time.AfterFunc` na nejbližší `run_at` aby idle worker nepojídal CPU.
  - Exponenciální backoff, respektuje `ctx.Done()` (drain inflight job + stop polling).
  - Per-job `traceID` (samostatný `uuid.NewV7`) propaguje se do logů a do `JobDispatcher.Enqueue(traceID, kind, payload)`.

- [ ] **Event integration**
  - `DispatchEventsMiddleware` zůstává — jen handlery, které chtějí queue, dostanou `JobDispatcher` v konstruktoru a samy zavolají `Enqueue`.
  - Rychlé/čisté handlery (např. log entry) běží dál synchronně.

- [ ] **CLI worker subcommand**
  - `./bin/app worker` — samostatný proces, jen worker pool (žádný HTTP server).
  - `./bin/app serve` ve výchozím stavu spouští in-process worker (vhodné pro single-binary deploy). Env flag `APP_INPROC_WORKER=false` to vypne pro split deployments (1× serve + N× worker).
  - V `RootCommand` registrovat nový `WorkerCommand`, v `container_provider.go` přidat provider.

- [ ] **`.go-arch-lint.yml`** — přidat komponenty `worker` (`infrastructure/worker`) a `job_application` (`application/job/**`). `domain/job/**` a `infrastructure/sqlite/job/**` pokrývají existující wildcardy.

### Definition of Done
- Test: `Enqueue(WelcomeEmail)`, kill worker procesu mid-execution, restart → job se claimne a doběhne.
- Test: handler vrátí error 3×, čtvrtý pokus dostane status `failed`, `last_error` obsahuje původní chybu.
- `make arch-check` projde.


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
  - Worker stejně — failed job po `max_attempts` jde do Sentry s `kind`, `payload` (truncated), `last_error`.

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
