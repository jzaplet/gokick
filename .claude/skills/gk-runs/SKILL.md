---
layout: 'page'
uri: '/skills/gk-runs'
position: 31
slug: 'skills-gk-runs'
parent: 'skills-data'
navTitle: 'gk-runs'
title: 'GK — Durable runs (agents)'
description: 'Perzistentní engine pro long-running práci („agenty"), která běží minuty/hodiny — běh MIMO transakci, checkpoint stavu, heartbeat lease, reclaim+resume z posledního checkpointu po pádu workeru, owner-fencing a cancel. Use when stavíš dlouho běžící background práci (AI agent, vícekrokový orchestrátor, dlouhý import) co musí přežít restart/crash a pokračovat tam, kde skončila — na rozdíl od krátkého fire-and-forget jobu.'
name: 'gk-runs'
---

# GK — Durable runs (agents)

Engine pro **dlouho běžící** background práci, která běží minuty až hodiny a musí
přežít pád procesu **bez ztráty postupu**. Na rozdíl od `/gk-jobs` (krátký job
obalený jednou transakcí) běží handler **MIMO transakci**, průběžně si **checkpointuje
stav**, heartbeat mu drží lease, a když worker umře, jiný worker job **převezme a
resumne z posledního checkpointu**. Viz ADR-0001.

## What & when

- Sáhni sem pro **long-running, resumovatelnou** práci: AI agent (LLM smyčka +
  nástroje), vícekrokový orchestrátor, dlouhý import/export. Cokoli, co běží dlouho
  a kde restart uprostřed nesmí začít od nuly.
- NEtýká se: krátké flaky práce (poslat mail, jedno API volání) → to je `/gk-jobs`
  (in-tx, atomický complete, levnější). Synchronní side-effect po commitu →
  `/gk-domain-events`. Periodická úloha na čase → `/gk-scheduler`.
- **Proč NE v jedné transakci:** dlouhý handler v `BEGIN IMMEDIATE` by držel
  globální SQLite write-lock celou dobu → zamrznou všechny zápisy appky (a na
  Postgresu je to idle-in-transaction anti-pattern). Proto outside-tx + checkpoint.

## For non-tech / juniors

Job (`/gk-jobs`) je **lísteček v krabici** — rychlá práce, buď celá projde, nebo
nic. Durable run je **dlouhá výprava s deníkem**: agent jde krok po kroku a po
každém kroku si do deníku (DB) zapíše, kde je. Když mu cestou umře baterka (spadne
proces), jiný průvodce vezme jeho deník a **pokračuje od posledního zápisu** — ne
od začátku. Aby se dva průvodci nehádali o stejnou výpravu, každý má **žeton**
(owner token): kdo žeton ztratí, ztratí právo do deníku zapisovat.

## How it works

**Schéma `runs`** (`migrations/…_create_runs_table.sql`) = jako `jobs` +
`state` (checkpoint BLOB), `locked_by` (owner token), `reclaims`, `updated_at`,
`cancel_requested`/`cancelled_at`. Stav se odvozuje ze sloupců (`completed_at` /
`failed_at` / `cancelled_at` != NULL → terminal; `locked_until > now` → běží).

**Entita + port** (`app/domain/run/`): `Run` + `Repository`. Dva oddělené čítače:
`Attempts` = **logické retry** (bumpuje jen `Reschedule`, cap `MaxRetries`),
`Reclaims` = **crash reclaimy** (bumpuje `ClaimDue` při převzetí vypršelého leasu,
cap zvlášť). Mutující metody jsou **owner-checked** a vrací `bool`: `false` = lease
ztracen → caller se vzdá (fencing). `ClaimDue`/`RenewLease`/`Checkpoint`/`MarkComplete`/
`Reschedule`/`MarkFailed`/`MarkCancelled`/`RequestCancel`/`IsCancelRequested` (levný
flag-only poll pro heartbeat, netáhne state BLOB)/`FindByID`. Čas: `julianday()` na
porovnání, ms-precizní zápisy, sub-second lease — viz `/gk-repositories`.

**Kontrakt** (`app/application/run/registry.go`): handler píše aplikace, ne gokick:
```go
func MyAgent(ctx context.Context, r *run.Run, ck run.Checkpointer) error {
    state := decode(r.State)         // resume z posledního checkpointu (len==0 = od začátku)
    for !state.Done {
        out := step(ctx, state)      // LLM/tool — MIMO tx, klidně sekundy; ctx je zrušitelný
        state = reduce(state, out)
        if err := ck.Save(ctx, encode(state)); err != nil {
            return err               // ErrLeaseLost = lease ztracen, přestaň
        }
    }
    return nil
}
```
`r` je **read-only** (resume z `r.State`/`r.Payload`, nemutuj ho). `ctx` se zruší
při ztrátě leasu / operátorském cancelu / shutdownu — handler musí být **ctx-aware**.

**Worker** (`app/infrastructure/worker/run_worker.go`, `RunWorker`): claim (per-claim
owner nonce) → handler v goroutině MIMO tx → vedle běží **heartbeat** (renew lease +
sleduje cancel) → finalize. **Backpressure** = `MaxInFlight` semafor. Při ztrátě
leasu / shutdownu se handler-ctx zruší a run se **abandonuje** (nikdy necompletuje
napůl hotovou práci) → lease vyprší → reclaim. Co-runs v `serve` i `worker` CLI vedle
job workeru.

**Dispatcher** (`app/application/run/dispatcher.go`, `shared.RunDispatcher`):
`RunDispatcherMiddleware` ho vloží do ctx; `Enqueue` z command handleru padne do
**stejné transakce** jako business write (atomický enqueue přes `Conn(ctx)`).

## Recipe

Přidání nového run kindu (vzorově `agent:summarize`):

1. **Handler** — `func(ctx, r *run.Run, ck run.Checkpointer) error` v
   `application/<context>/run/`. Resumuj z `r.State`, checkpointuj přes `ck.Save`,
   respektuj `ctx`. Musí být **idempotentní** (krok se po pádu může zopakovat).
2. **Registruj kind** v `provideRunHandlerRegistry` (`container_provider.go`):
   `"agent:summarize": {Handler: h.Handle}` (volitelně `Lease:` pro per-kind lease).
3. **Enqueue z command handleru** — po business write:
   ```go
   return shared.RunDispatcherFromContext(ctx).
       Enqueue(ctx, "agent:summarize", 3, SummarizePayload{DocID: id})
   ```
   `maxRetries` je povinný (per-kind rozhodnutí). Odložení: `shared.WithDelay(...)`.
4. **`make di`** — přegeneruj Wire. Hotovo.

## Invariants & pitfalls

- **Run handler NESMÍ otevřít transakci — vynuceno.** Dlouhý handler v `BEGIN IMMEDIATE`
  by držel globální SQLite write-lock celou dobu běhu → freeze všech zápisů. Worker proto
  označí handler ctx `shared.ContextForbidTx`; `Transactor.BeginTx` v té zóně **fail-closed
  selže** (+ statická `zz_notx_test.go` brána skenuje run path). Stav perzistuj přes
  `Checkpointer`; transakční side-work **zařaď jako command/job** (poběží ve vlastní krátké
  tx mimo run). Kdy run vs job vs scheduler vs event → [[gk-jobs]] / `docs/framework/background-work.md`.
- **Resumovatelnost je na tobě.** Mimo tx zaniká atomicita „handler-writes + complete"
  — handler musí být idempotentní a resumovatelný z `r.State`. Placené LLM/tool akce
  zaznamenej před efektem nebo udělej idempotentní (at-least-once + okno zdvojení
  během handoffu).
- **`lease ≫ heartbeat interval`.** Jinak ve špičce hrozí falešná expirace → run
  poběží dvakrát. Defaulty: lease 5 min, heartbeat = lease/3 (config `APP_RUN_WORKER_*`).
- **`attempts` ≠ `reclaims`.** Crash (deploy/OOM) nesmí spálit retry budget dlouhého
  runu — proto se počítají zvlášť; `MaxReclaims` chrání před poison-crash-loopem
  (kontrola hned po claimu, před handlerem).
- **Cancel je dvoufázový.** `RequestCancel` jen nastaví signal (run běží dál, jede na
  řádku → **přežije reclaim**); worker ho zachytí, ukončí handler a zapíše `cancelled_at`.
- **Backpressure + paměť.** `MaxInFlight` limituje souběh (paměť na agenta!) — zbytek
  čeká ve frontě. Go zvládne hodně goroutin; strop je paměť + SQLite jeden writer.
- **SQLite je dev/malý provoz.** Strop ≈ ~8000 owner-checked zápisů/s (viz load test
  `sqlite_loadtest_test.go`). Na desítky tisíc agentů / častý velký checkpoint →
  Postgres (roadmap krok 7), stejné porty → výměna adaptéru, bez přepisu handleru.
- **Z handleru NEvolat `EventCollector.Collect`** (jako u jobů) — pro navazující async
  práci zařaď další run/job.

## Related

- Sousední skills: `/gk-jobs` (krátký in-tx job), `/gk-domain-events`,
  `/gk-repositories` (owner-fence, julianday, ms-precision), `/gk-feature`, `/gk-config`.
- ADR: durable execution model (outside-tx + lease/heartbeat/checkpoint), škála SQLite vs Postgres.
- Kód: `app/domain/run/`, `app/application/run/`,
  `app/infrastructure/sqlite/run/repository.go`, `app/infrastructure/worker/run_worker.go`,
  `app/domain/shared/run_dispatcher.go`, `app/presentation/console/serve.go`.
