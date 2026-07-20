---
layout: 'page'
uri: '/skills/gk-runs'
position: 31
slug: 'skills-gk-runs'
parent: 'skills-data'
navTitle: 'gk-runs'
title: 'GK — Durable runs'
description: 'Jeden perzistentní engine pro background práci, co běží MIMO transakci a musí přežít restart/crash — ve dvou tvarech. Fire-and-forget run (FireAndForget: krátké, at-least-once, idempotentní — poslat mail/SMTP, webhook, jedno volání cizího API) a durable run (Durable: checkpoint stavu + resume z posledního kroku po pádu — velký import/export, generování velkého reportu, dávkové zpracování). Use when stavíš jakoukoli background práci mimo HTTP request — od krátkého fire-and-forget runu po dlouhý běh na minuty/hodiny — nebo řešíš retry/backoff, lease/heartbeat, owner-fencing nebo cancel.'
name: 'gk-runs'
---

# GK — Durable runs

**Jeden** engine pro perzistentní background práci, která běží **mimo transakci**
a musí přežít pád procesu. Má **dva tvary**, lišící se jen registrací:

- **fire-and-forget run** (`FireAndForget`) — krátká práce, co **nečekpointuje**:
  pošli mail/SMTP, zavolej webhook nebo cizí API, přepočítej agregaci. Default lease,
  doručení **at-least-once** → handler musí být idempotentní.
- **durable run** (`Durable`) — práce na minuty až hodiny, co si **checkpointuje
  stav** a po pádu workeru se **resumne z posledního checkpointu**: velký import/export,
  generování velkého reportu/PDF, dávkové zpracování.

Oba tvary jedou přes stejnou tabulku `runs`, stejný worker, stejné lease/heartbeat,
owner-fencing a cancel. Fire-and-forget run je prostě durable run bez checkpointu.

## What & when

- Sáhni sem pro **jakoukoli práci mimo HTTP request**, co musí přežít restart/crash:
  - **krátká, flaky, volá ven** (mail/SMTP, webhook, jedno API volání) nebo rychlá
    práce nad vlastní DB → **fire-and-forget run** (`FireAndForget`).
  - **dlouhá a nesmí začít od nuly**, kdyby proces spadl (import, report, dávka)
    → **durable run** (`Durable`, checkpoint + resume).
- NEtýká se: synchronního side-effectu po commitu v request goroutině (rychlá reakce
  na „stalo se X") → `/gk-domain-events`. Periodická úloha na čase (každou hodinu
  cleanup) → `/gk-scheduler`. Audit log má vlastní cestu (`AuditCollector`).
- **Proč VŠECHNO běží mimo transakci:** dlouhý handler v `BEGIN IMMEDIATE` by držel
  globální SQLite write-lock celou dobu → zamrznou všechny zápisy appky (a na Postgresu
  je to idle-in-transaction anti-pattern). A i **krátké volání ven** (SMTP visí, API
  i minuty) v transakci drží zámek po dobu toho volání. Proto je outside-tx vynucené pro
  oba tvary — atomicitu „práce + complete" nahrazuje idempotence (fire-and-forget run) / checkpoint (durable run).

## For non-tech / juniors

Představ si **krabici úkolů**, kterou hlídá zvlášť běžící pracant (`worker`).

- **Fire-and-forget run = lísteček „udělej a zapomeň".** Pošli welcome mail tomuhle uživateli. Pracant
  lísteček vezme, udělá práci, a když se povede, škrtne ho. Když se nepovede (SMTP
  nedostupný), počká a zkusí znovu, s každým pokusem déle. Lísteček je v databázi, takže
  **přežije i pád aplikace**. Protože škrtnutí „hotovo" je **samostatný zápis až po práci**,
  může se po pádu práce ve vzácném okně zopakovat — proto musí být **idempotentní** (poslat
  dva maily je bug).
- **Run = dlouhá výprava s deníkem.** Práce jde krok po kroku a po každém kroku si do
  deníku (DB) zapíše, kde je. Když cestou umře baterka (spadne proces), jiný průvodce
  vezme jeho deník a **pokračuje od posledního zápisu** — ne od začátku. Aby se dva
  průvodci nehádali o stejnou výpravu, každý má **žeton** (owner token): kdo žeton ztratí,
  ztratí právo do deníku zapisovat.

Fire-and-forget run je výprava bez deníku (jeden krok, žádný checkpoint). Pod kapotou je to ten samý engine.

## How it works

**Schéma `runs`** (`migrations/…_create_runs_table.sql`) = fronta + `state`
(checkpoint BLOB), `locked_by` (owner token), `reclaims`, `parks`, `updated_at`,
`cancel_requested`/`cancelled_at`. Stav se odvozuje ze sloupců (`completed_at` /
`failed_at` / `cancelled_at` != NULL → terminal; `locked_until > now` → běží).

**Entita + port** (`app/domain/run/`): `Run` + `Repository`. Tři oddělené čítače:
`Attempts` = **logické retry** (bumpuje jen `Reschedule`, cap `MaxRetries`),
`Reclaims` = **crash reclaimy** (bumpuje `ClaimDue` při převzetí vypršelého leasu,
cap zvlášť), `Parks` = **registry-skew parky** (bumpuje `Park` u neznámého kindu,
cap zvlášť). Mutující metody jsou **owner-checked** a vrací `bool`: `false` = lease
ztracen → caller se vzdá (fencing). `ClaimDue`/`RenewLease` (heartbeat; v témže owner-checked zápisu vrací přes
`RETURNING` i `cancel_requested` — žádný samostatný cancel-poll, state BLOB se
netáhne)/`Checkpoint`/`MarkComplete`/`Reschedule`/`Park`/`MarkFailed`/`MarkCancelled`/
`RequestCancel`/`FindByID`. Čas: `julianday()` na
porovnání, ms-precizní zápisy, sub-second lease — viz `/gk-repositories`.

**Kontrakt + registrace** (`app/application/run/registry.go`): handler píše aplikace,
ne gokick. Stejná signatura pro oba tvary; liší se jen **registrace**:

```go
func RunMyWork(ctx context.Context, r *run.Run, ck run.Checkpointer) error {
    state := decode(r.State)         // resume z posledního checkpointu (len==0 = od začátku)
    for !state.Done {
        out := step(ctx, state)      // jeden krok — MIMO tx, klidně sekundy; ctx je zrušitelný
        state = reduce(state, out)
        if err := ck.Save(ctx, encode(state)); err != nil {
            return err               // ErrLeaseLost = lease ztracen, přestaň
        }
    }
    return nil
}
```

```go
// registrace (provideRunHandlerRegistry, container_provider.go):
"welcome:send": runapp.FireAndForget(h.SendWelcome, 30*time.Second),   // fire-and-forget run: bez leasu, jen timeout
"import:bulk":  runapp.Durable(h.RunImport, 5*time.Minute, time.Hour), // durable run: lease + timeout
```

`FireAndForget(handler, timeout)` = default lease, **handler nemusí volat `ck.Save`**.
`Durable(handler, lease, timeout)` = explicitní crash-reclaim lease, handler checkpointuje.
`r` je **read-only** (resume z `r.State`/`r.Payload`, nemutuj ho). `ctx` se zruší při
ztrátě leasu / operátorském cancelu / shutdownu — handler musí být **ctx-aware**.

**Worker** (`app/infrastructure/worker/run_worker.go`, `RunWorker`): claim (per-claim
owner nonce) → handler v goroutině MIMO tx (`shared.ContextForbidTx`) → vedle běží
**heartbeat** (renew lease + sleduje cancel) → finalize. Worker nevětví podle tvaru:
oba tvary jdou stejnou cestou, jen fire-and-forget run typicky nezavolá `ck.Save` a doběhne na jeden
průchod. **Backpressure** = `MaxInFlight` semafor. Při ztrátě leasu / shutdownu se
handler-ctx zruší a run se **abandonuje** (nikdy necompletuje napůl hotovou práci) →
lease vyprší → reclaim. Co-runs v `serve` i `worker` CLI (`./bin/app worker` bez HTTP
serveru pro split deploy: 1× serve + N× worker, sdílená SQLite).

**Dispatch** (`app/application/run/dispatcher.go`, `shared.RunDispatcher`) — **stejný
pro oba tvary**: `RunDispatcherMiddleware` ho vloží do ctx; `Enqueue` z command handleru
padne do **stejné transakce** jako business write (atomický enqueue přes `Conn(ctx)`).
Neznámý `kind` (bez registrovaného handleru) i `maxRetries < 0` selžou už při enqueue.
Mimo bus (testy, přímé volání handleru) vrací no-op dispatcher — enqueue se tiše
zahodí, handler nenil-checkuje. CLI příkazy jedou přes SystemCommandBus, který
RunDispatcher nese — enqueue z CLI je durable INSERT a run si vyzvedne serve/worker.

## Recipe

Přidání nového kindu (vyber tvar podle „potřebuje checkpoint/resume?"):

1. **Handler** — `func(ctx, r *run.Run, ck run.Checkpointer) error` v
   `application/<context>/run/`. Musí být **idempotentní** (po pádu se zopakuje).
   - *Fire-and-forget run*: udělej práci, `return nil`/`err`. `ck.Save` nepotřebuješ.
   - *Durable run*: resumuj z `r.State`, checkpointuj přes `ck.Save`, respektuj `ctx`.
2. **Registruj kind** v `provideRunHandlerRegistry` (`container_provider.go`):
   ```go
   "welcome:send": runapp.FireAndForget(h.SendWelcome, 30*time.Second), // fire-and-forget run
   "import:bulk":  runapp.Durable(h.RunImport, 5*time.Minute, time.Hour), // durable run
   ```
3. **Enqueue z command handleru** — po business write (identické pro oba tvary):
   ```go
   return shared.RunDispatcherFromContext(ctx).
       Enqueue(ctx, "welcome:send", 0, WelcomePayload{UserID: u.ID, Email: u.Email})
   ```
   `maxRetries` je povinný poziční parametr (per-kind rozhodnutí, dispatcher odmítne
   záporný): `0` = jednou bez retry, `3` = až 3 retry po prvním selhání. Odložení:
   `shared.WithDelay(time.Hour)` jako další arg.
4. **`make di`** — přegeneruj Wire. Hotovo.

## Invariants & pitfalls

- **Handler NESMÍ držet transakci přes pomalou práci — implicitní tx je vynuceně
  zakázaná.** Worker označí handler ctx `shared.ContextForbidTx`; `Transactor.BeginTx`
  v té zóně **fail-closed selže** (+ statická `zz_notx_test.go` brána skenuje run path).
  Dlouhý handler v `BEGIN IMMEDIATE` by držel globální SQLite write-lock → freeze;
  i krátké volání ven by drželo zámek po dobu volání. Stav perzistuj přes `Checkpointer`;
  pár atomických zápisů udělej přes `shared.WithTx(ctx, fn)` — krátkou tx, kterou si
  handler sám ohraničí (worker mu `Transactor` do ctx injektuje; drž ji krátkou, žádné
  pomalé/externí I/O uvnitř — přesně jako v command handleru). Vnořený `WithTx` i syrový
  `BeginTx` uvnitř fail-closed selžou. Kdy durable run vs fire-and-forget run vs
  scheduler vs event → `docs/framework/background/overview.md`.
- **At-least-once → idempotence VŠEHO.** Mimo tx zaniká atomicita „handler-writes +
  complete" — `MarkComplete` je **samostatný zápis až po návratu handleru**, takže crash
  v okně mezi nimi handler na reclaimu **zopakuje**. Idempotentní musí být nejen externí
  efekty (poslat dva maily je bug), ale **i DB writes** handleru (žádný in-tx rollback je
  už neochrání). U runu navíc resumuj z `r.State`.
- **`lease ≫ heartbeat interval`.** Jinak ve špičce hrozí falešná expirace → práce poběží
  dvakrát. Defaulty: lease 5 min, heartbeat = lease/3 (config `APP_RUN_WORKER_*`). Per-kind
  lease přes `Durable(...)`; sub-ms lease/timeout je odmítnut už při bootu (units typo).
- **`attempts` ≠ `reclaims`.** Crash (deploy/OOM) nesmí spálit retry budget — počítají se
  zvlášť; `MaxReclaims` chrání před poison-crash-loopem (kontrola hned po claimu, před
  handlerem). Vyčerpané retry/reclaim → `MarkFailed` + **report do Sentry**.
- **Cancel je dvoufázový.** `RequestCancel` jen nastaví signal (run běží dál, jede na řádku
  → **přežije reclaim**); worker ho zachytí, ukončí handler a zapíše `cancelled_at`.
- **Backpressure + paměť.** `MaxInFlight` limituje souběh (paměť na jeden běh!) — zbytek
  čeká ve frontě. Go zvládne hodně goroutin; strop je paměť + SQLite jeden writer.
- **SQLite je dev/malý provoz.** Strop ≈ ~8000 owner-checked zápisů/s (viz load test
  `sqlite_loadtest_test.go`). Na desítky tisíc souběžných běhů / častý velký checkpoint →
  Postgres (roadmap krok 7), stejné porty → výměna adaptéru, bez přepisu handleru.
- **Z handleru NEvolat `EventCollector.Collect`.** Sběrač eventů se vyprazdňuje jen v
  command request goroutině; ve workeru je zablokovaný a Collect je runtime panic. Pro
  navazující async práci zařaď další run (`RunDispatcher` je v ctx).
- **Neznámý kind = park s backoffem, ne okamžitý fail.** Worker dostane kind bez handleru
  (deploy/registry-skew) → `Park`: reschedule s exponenciálním backoffem, bumpne vyhrazený
  čítač `parks` (NE `attempts` — deploy okno nespálí logic-retry budget, přežije ho i run-once
  run s `maxRetries=0`). Teprve po vyčerpání park budgetu (`minUnknownKindParks` = 5)
  → `MarkFailed` + report do Sentry.

## Related

- Sousední skills: `/gk-domain-events` (synchronní side-effect po commitu),
  `/gk-scheduler` (periodicky na čase), `/gk-repositories` (owner-fence, julianday,
  ms-precision), `/gk-bus` (middleware chain — kam padá enqueue), `/gk-feature`,
  `/gk-config` (DI registrace).
- Docs: [Durable run](/framework/durable-run), [Fire-and-forget](/framework/fire-and-forget) (fire-and-forget
  tvar), [Background work](/framework/overview) (co kdy + proč vše mimo tx).
- Kód: `app/domain/run/`, `app/application/run/` (`registry.go` = `FireAndForget`/`Durable`,
  `dispatcher.go`), `app/infrastructure/sqlite/run/repository.go`,
  `app/infrastructure/worker/run_worker.go`, `app/domain/shared/run_dispatcher.go`,
  `app/presentation/console/serve.go`.
