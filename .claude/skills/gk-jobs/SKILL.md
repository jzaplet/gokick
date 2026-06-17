---
name: gk-jobs
description: Perzistentní fronta pro background práci, která musí proběhnout i když proces mezitím spadne — atomický enqueue v transakci, worker s retry/backoff, mark-complete v handlerově tx. Use when chceš spustit pomalou nebo nespolehlivou práci náchylnou k opakování (poslat mail přes SMTP, volat externí API) mimo HTTP request a potřebuješ, aby přežila restart i pád procesu.
layout: 'page'
uri: '/skills/gk-jobs'
position: 30
slug: 'skills-gk-jobs'
parent: 'skills-data'
navTitle: 'gk-jobs'
title: 'GK — Job queue'
---

# GK — Job queue

Perzistentní fronta background práce: SQLite tabulka `jobs`. Command handler
zapíše job **ve stejné transakci** jako business write, worker si ho atomicky
vyzvedne, spustí v transakci a při selhání naplánuje retry s exponenciálním
backoffem. Když spadne celý proces, po restartu se pokračuje.

## What & when

- Sáhni sem pro **pomalou nebo selhávající práci**, co musí přežít restart/crash:
  mail přes SMTP, volání externího API, cokoli retry-prone. Job se uloží do DB a
  nezávisí na životě HTTP requestu.
- NEtýká se: synchronního side-efektu v request goroutině (rychlá reakce na
  „stalo se X") → to je `/gk-domain-events`. Periodické úlohy na čase (každou
  hodinu cleanup) → `/gk-scheduler`. Audit log má vlastní cestu (`AuditCollector`).

## For non-tech / juniors

Představ si **lísteček v krabici úkolů**. Když uživatel něco udělá (založí účet),
neposíláme welcome mail hned v rámci jeho requestu — to by ho zdrželo a kdyby
SMTP server zlobil, spadlo by mu to celé. Místo toho do krabice hodíme lísteček
„pošli welcome mail tomuhle uživateli". Lísteček a samotné založení účtu jdou do
databáze **najednou** — buď obojí, nebo nic.

Zvlášť běžící pracant (`worker`) krabici pravidelně kouká. Vezme jeden lísteček,
udělá práci, a teprve když se povede, lísteček škrtne jako hotový. Když se práce
nepovede (SMTP nedostupný), lísteček nezahodí — počká chvíli a zkusí to znovu, s
každým pokusem déle. A protože lístečky jsou v databázi, **přežijí i pád aplikace**
— po restartu pracant pokračuje tam, kde skončil.

## How it works

**Entita `Job`** (`app/domain/job/job.go`) — stav se odvozuje ze sloupců, ne z enumu:
`completed_at != nil` → hotovo, `failed_at != nil` → trvale selhalo (vyčerpané
retry), `locked_until > now` → právě se zpracovává, jinak → pending/retryable.
`NewJob(kind, payload, maxRetries)` vytvoří pending job s UUIDv7 a `RunAt=now`.

**Repository port** (`app/domain/job/repository.go`), implementace v
`app/infrastructure/sqlite/job/repository.go`:

- `Enqueue` — INSERT přes `Conn(ctx)`, takže uvnitř command handleru padne do
  **stejné transakce** jako business write (atomicita).
- `ClaimDue` — jádro celého návrhu: **atomický claim** jediným `UPDATE … RETURNING`.
  Vybere jeden due řádek (`run_at <= now`, nezamčený, nehotový), bumpne `attempts`
  a nastaví `locked_until = now + lockFor`. SQLite serializuje writery, takže dva
  workeři nikdy nedostanou stejný řádek. Index `idx_jobs_claim`
  (`migrations/20260517000001_create_jobs_table.sql`) claim podpírá.
- `MarkComplete` / `Reschedule` / `MarkFailed` — finální stavy.

**Dispatcher** (`app/application/job/dispatcher.go`, implementuje
`shared.JobDispatcher` z `app/domain/shared/job_dispatcher.go`) — outbox-style
enqueue. `Enqueue(ctx, kind, maxRetries, payload, opts...)`:

1. ověří `maxRetries >= 0`,
2. ověří, že `kind` je v `HandlerRegistry` (`registry.Has` — překlep chytíš v testu,
   ne v produkci),
3. payload zmarshaluje do JSON, vytvoří `Job` a zavolá `repo.Enqueue`.

Handler ho dostane v ctx přes `shared.JobDispatcherFromContext(ctx)`. Mimo bus
(CLI, testy) vrací no-op dispatcher — enqueue se tiše zahodí, handler nemusí
nil-checkovat. Do command handleru ho vkládá `JobDispatcherMiddleware`
(`app/application/bus/middleware/job_dispatcher.go`), který sedí **vně**
`TransactionMiddleware`, takže Enqueue uvnitř handleru běží přes `Conn(ctx)` =
ta samá transakce.

**HandlerRegistry** (`app/application/job/registry.go`) — mapa `kind → HandlerFunc`
(`func(ctx, payload []byte) error`). Plní se jednou při DI v
`provideJobHandlerRegistry` (`container_provider.go`). **Pozn.: dnes je registr
prázdný** — žádný reálný job handler zatím není zaregistrovaný; infrastruktura
stojí, čeká na první kind.

**Worker** (`app/infrastructure/worker/worker.go`, `NewWorker(...)`) — pool
goroutin (default concurrency `1`, `provideWorker`), každá v `loop` po
`defaultPollInterval` (`1s`) volá `processOne`:

1. `ClaimDue` (lock na `defaultLockFor` = `5min`),
2. neznámý kind → `MarkFailed` + report do `ErrorReporter` (registry-skew, žádný retry),
3. `runWithinTx`: otevře tx, vloží dispatcher do ctx, **zablokuje `EventCollector`**
   (`ContextWithoutEventCollector` — cascade event z workeru je runtime panic),
   spustí handler. Když handler vrátí `nil`, zavolá `MarkComplete` **uvnitř téže
   transakce** a tu potvrdí (commit). Když handler vrátí chybu nebo vyvolá paniku → **rollback celé transakce**
   (i DB writes handleru), job zůstává claimovatelný.
4. `handleFailure`: pokud `Attempts > MaxRetries` → `MarkFailed` + report; jinak
   `Reschedule` s `backoff(attempts)` = `2^(attempts-1) * 5s`, cap `1h`.

**Spuštění:** worker běží uvnitř `./bin/app serve` (vedle HTTP serveru a
scheduleru, sdílí ctx — `serve.go`), nebo samostatně `./bin/app worker` bez HTTP
serveru (`app/presentation/console/worker.go`) pro split deploy
(1× serve + N× worker, sdílená SQLite).

## Recipe

Přidání nového job kindu (vzorově `welcome:send` — pošli mail po `CreateUser`):

1. **Handler funkce** — `func(ctx, payload []byte) error`. Unmarshaluj `payload`
   do svého structu, udělej práci. `return err` → retry; `return nil` → hotovo.
   Patří do `application/<context>/job/` (např. `application/user/job/send_welcome.go`).
2. **Zaregistruj kind** v `provideJobHandlerRegistry` (`container_provider.go`):
   přidej `"welcome:send": welcomeHandler.Handle` do mapy a vlož handler do
   signatury providera.
3. **Enqueue z command handleru** — po business write:
   ```go
   if err := h.users.Save(ctx, u); err != nil { return err }
   return shared.JobDispatcherFromContext(ctx).
       Enqueue(ctx, "welcome:send", 0, WelcomePayload{UserID: u.ID, Email: u.Email})
   ```
   `maxRetries` je povinný poziční parametr: `0` = vykonej jednou bez retry,
   `3` = až 3 retry po prvním selhání (4 pokusy celkem). Odložené spuštění:
   `shared.WithDelay(time.Hour)` jako další arg.
4. **`make di`** — přegeneruj Wire. Hotovo.

## Invariants & pitfalls

- **Atomicita business write + enqueue.** Enqueue z command handleru běží přes
  `Conn(ctx)` a `JobDispatcherMiddleware` je vně transakce — INSERT padne do téže
  transakce. Buď se uloží obojí, nebo nic.
- **At-least-once.** Job proběhne **minimálně** jednou. Handler musí být
  **idempotentní pro externí side effects** — poslat dva maily je bug. DB writes
  se vrátí zpět (rollback) společně s mark-complete flagem, takže o jejich idempotenci
  přemýšlet nemusíš.
- **Mark-complete v handlerově transakci.** „Job hotový" se uloží (commit) ve stejné transakci jako
  handlerovy DB writes. Crash mezi nimi je nemožný — žádné částečné stavy.
- **`maxRetries` se nesmí defaultovat.** Je to per-kind rozhodnutí; dispatcher
  odmítne záporné a `NewJob` to dokumentuje. Vol vědomě.
- **Z job handleru NEvolat `EventCollector.Collect`.** Sběrač eventů se vyprazdňuje
  jen v command request goroutině; ve workeru je zablokovaný a Collect je runtime
  panic. Pro navazující async práci zařaď další job (`JobDispatcher` je v ctx).
- **Neznámý kind = terminal failure.** Když worker dostane kind, co nemá v
  registru (deploy/registry-skew), označí job `failed` a reportuje do Sentry —
  nezkouší retry.
- **Default 1 worker.** SQLite serializuje writery (WAL: jeden writer na DB). Víc
  goroutin nezvýší throughput DB-bound handlerů; má smysl jen u I/O-bound práce
  mimo SQLite.

## Related

- Sousední skills: `/gk-domain-events` (synchronní side-effect po commitu),
  `/gk-bus` (middleware chain, pořadí transakce/eventů/jobů), `/gk-feature`
  (přidání featury end-to-end), `/gk-config` (DI registrace providerů).
- Kód: `app/domain/job/`, `app/application/job/`,
  `app/infrastructure/sqlite/job/repository.go`, `app/infrastructure/worker/worker.go`,
  `app/application/bus/middleware/job_dispatcher.go`,
  `app/domain/shared/job_dispatcher.go`,
  `app/presentation/console/worker.go`, `app/presentation/console/serve.go`,
  `migrations/20260517000001_create_jobs_table.sql`
