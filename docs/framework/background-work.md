---
layout: 'page'
uri: '/framework/background-work'
position: 45
slug: 'framework-background-work'
parent: 'framework'
navTitle: 'Background work'
title: 'Background work — co kdy použít'
description: 'Rozhodovací matice pro background práci: command vs doménový event vs job vs durable run vs scheduler — a klíčová transakční hranice (jen runy běží mimo transakci, vynuceně, aby dlouhý handler nezamkl celou SQLite).'
---

# Background work — co kdy použít

gokick má pět mechanismů, jak něco „udělat". Liší se hlavně ve dvou věcech: **běží to v transakci?** a **kdy/kde to běží?** Špatná volba buď ztratí práci (něco, co mělo přežít restart, běželo inline), nebo **zamkne celou databázi** (dlouhý handler v transakci drží globální SQLite write-lock).

![Rozhodovací matice background práce + transakční hranice](files/background-work.svg)

> Detailní toky: [Command flow](/framework/command-flow) · [Event flow](/framework/event-flow) · [Job flow](/framework/job-flow) · [Run flow](/framework/run-flow) · [Scheduler flow](/framework/scheduler-flow). Návody: `/gk-commands`, `/gk-domain-events`, `/gk-jobs`, `/gk-runs`, `/gk-scheduler`.


## Rozhodovací matice

| Mechanismus | V transakci? | Kdy / kde běží | Použij na | Cena |
|---|---|---|---|---|
| **Command** | ✅ ano (`TransactionMiddleware`) | synchronně během requestu | atomická write operace (vytvoř/uprav/smaž) | drží request |
| **Doménový event** | ❌ ne — **po commitu**, synchronně | request lifecycle, po commitu | „stalo se X" → reakce, kterou command nezná | pomalý handler blokuje response |
| **Job** | ✅ ano — `runWithinTx` (krátká, atomický complete) | worker, async | krátká nespolehlivá práce (mail, API), musí přežít restart | at-least-once (idempotence) |
| **Durable run** | ❌ **NE — vynuceně outside-tx** | worker, async, **dlouhý** (minuty–hodiny) | long-running „agent" (LLM smyčka), checkpoint + resume po pádu | resumovatelnost na tobě |
| **Scheduler Fn** | ❌ ne (inline na tickeru) | periodicky, in-process | cron-like údržba (cleanup, sync) | bez leader election → na víc instancích běží víckrát |


## Transakční hranice (proč to je klíčové)

SQLite má **jediného writera**. Transakce se otevírá `BEGIN IMMEDIATE` (DSN `_txlock=immediate`), takže **vezme globální write-lock hned na začátku** a drží ho do konce transakce. Z toho plyne tvrdé pravidlo:

- **Command / Job** transakci **mají chtít** — jsou **krátké**. Atomicita (business zápis + enqueue jobu, resp. handler + mark-complete) za cenu krátkého držení locku.
- **Durable run** transakci **NESMÍ mít**. Dlouhý handler (agent běžící minuty/hodiny) v `BEGIN IMMEDIATE` by držel globální write-lock celou tu dobu → **zamrznou všechny ostatní zápisy** (commandy, joby, jiné runy → `busy_timeout` → 500). Proto run běží mimo transakci a stav ukládá **krátkými checkpoint zápisy**; heartbeat drží lease, po pádu jiný worker resumne z posledního checkpointu (durable execution à la Temporal/Inngest). Viz ADR-0001 (durable agent execution model).
- **Doménový event** běží **až po commitu** (middleware `DispatchEvents` obaluje `Transaction`), synchronně, **bez ambient transakce** — pomalý event handler tedy blokuje HTTP response. Pro těžkou návaznou práci event handler **zařadí job/run**, nedělá ji inline.
- **Scheduler Fn** běží inline na tickeru, bez ambient transakce. Lehký cleanup v krátké transakci je OK; **těžkou periodickou práci má Fn zařadit jako job/run**, ne ji dělat inline.

### Vynucení „run nesmí otevřít transakci"

Aby dlouhý run handler nemohl omylem zamknout DB, je pravidlo **vynucené dvěma cestami**:

1. **Runtime fail-closed guard.** Run worker označí handler ctx přes `shared.ContextForbidTx`; `Transactor.BeginTx` (`SqliteManager`) v té zóně **selže s jasnou chybou** místo otevření transakce. Chytí i app-level handlery (choke-point je `BeginTx`, dosažitelný přes injektovaný doménový `Transactor`). Analogie multitenancy fail-closed `r.Tenant(ctx)`.
2. **Statická conformance brána** (`zz_notx_test.go`). Skenuje run execution path (run worker + `application/run`) na transakční syntaxi a **selže build**. (go-arch-lint to tu neumí: run worker sdílí balíček s job workerem, který transakci legitimně používá, a tx se injektuje přes doménový interface — import-level pravidlo by run worker ani neizolovalo, ani závislost nevidělo. Source-scan dělá obojí.)

Run handler tedy stav perzistuje přes **`Checkpointer`**; pro transakční „side-work" **zařadí command/job**, který běží ve vlastní krátké transakci mimo run.


## Rychlá volba

- Musí to být **atomické s requestem**? → **command**.
- „**Stalo se X**", ať na to někdo zareaguje, aniž to command zná? → **doménový event** (ale rychlý; těžké → enqueue).
- **Krátká** nespolehlivá práce, co musí **přežít restart**? → **job**.
- **Dlouho běžící** práce (agent), co musí **resumnout po pádu**? → **durable run** (mimo tx!).
- **Periodicky** (cron-like)? → **scheduler** (těžké → enqueue job/run).


## Související

- Toky: [Command flow](/framework/command-flow), [Event flow](/framework/event-flow), [Job flow](/framework/job-flow), [Run flow](/framework/run-flow), [Scheduler flow](/framework/scheduler-flow).
- Skilly: `/gk-commands`, `/gk-domain-events`, `/gk-jobs`, `/gk-runs`, `/gk-scheduler`, `/gk-bus`.
- ADR-0001 — durable agent execution model (zatím mimo repo).
