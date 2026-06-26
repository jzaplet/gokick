---
layout: 'page'
uri: '/briefs/0001-durable-task-convergence'
position: 2
slug: 'briefs-0001-durable-task-convergence'
parent: 'briefs'
navTitle: 'Plán: sloučit job + run'
title: 'Plán: sloučit job + durable run do jednoho primitiva'
description: 'Job handler drží SQLite write-lock po celou dobu běhu (runWithinTx), takže „volání ven v jobu" zamrzne DB — a po opravě toho footgunu se job a run skoro slejou. Návrh sloučit je do jednoho durable-task primitiva (mimo tx, idempotentní, volitelný checkpoint, timeout).'
---

# Plán: sloučit job + durable run do jednoho primitiva

**Status:** 🟢 směr rozhodnut (sloučit) · **Fáze:** follow-up po `feature/durable-runs` (PR #21) · **Souvisí s ADR:** ADR-0001 (durable execution model)

> Samostatný PR **po** smerge PR #21. Nepřilepovat na #21 (ten je „durable runs" + no-tx enforcement, hotový a v revizi).


## Problém

Job handler běží **celý uvnitř transakce** (`worker.runWithinTx`: `BeginTx → handler → MarkComplete → Commit`). Protože SQLite bere write-lock hned na `BEGIN IMMEDIATE`, drží se zámek na zápis **po celou dobu běhu handleru**. Z toho:

1. **Footgun:** kanonický příklad jobu „poslat e-mail / zavolat cizí API" je vlastně chyba — SMTP může viset, API request trvá minuty → po tu dobu **nikdo nic nezapíše** (zamrzne celá DB). A toto pravidlo („nevolej ven v jobu") **nejde vynutit** (SMTP/HTTP v Go nedetekuješ); timeout freeze jen ohraničí, neodstraní.
2. **Skoro-duplicita:** durable run ([Run flow](/framework/run-flow)) řeší „mimo tx + at-least-once + idempotence" a pravidlo „mimo tx" **vynutit umí** (`shared.ContextForbidTx` → `BeginTx` fail-closed). Po opravě footgunu (job = jen rychlá lokální DB práce) je rozdíl job vs run jen: job restartuje od nuly + je levný (bez heartbeatu/checkpointu); run resumuje z checkpointu. **Run, který necheckpointuje, JE job.**

Pro boilerplate (čistý vzor) = nevynutitelný footgun + skoro-duplicitní mechanismus = špatný obchod.


## Varianty

| Varianta | Pro | Proti |
|----------|-----|-------|
| **A — nechat dva, otužit joby** | zachová atomický „handler-DB-write + complete"; menší změna | „nevolej ven" zůstane nevynutitelný footgun; matice „job vs run" mate; dvě skoro-stejné věci |
| **B — sloučit do jednoho ✅** | jeden mechanismus, bez nevynutitelné pasti; zmizí matice; pravidlo „mimo tx" platí všude a je vynutitelné | větší změna; ztratí se atomický „handler-write + complete" (nahradí idempotence — tu stejně potřebuješ) |

Volba: **B.** Rozhodující je vynutitelnost — runové „mimo tx" je fail-closed, jobové „nevolej ven" nikdy nebude.

**Otevřená nika k ověření:** existuje práce, co je async + JEN nad DB + potřebuje atomicitu-s-dokončením a *ne*jde idempotentní? Pokud ne (synchronní commandy ji pokryjou), B nemá co ztratit.


## Návrh

Jedno primitivum **`durable task`** — dnešní run rozšířený o levný „bez checkpointu" mód:

- Běží **mimo transakci** (vynuceno přes `ContextForbidTx`), **idempotentní / at-least-once**.
- **Volitelný `Checkpointer`** — kdo checkpointuje, dostane resume po pádu (dnešní run); kdo ne, restartuje od nuly (dnešní job).
- **Timeout per task** (context deadline na handler) — chybí dnes i u jobů; killne zaseknutý task.
- Zachovat z runu: lease + heartbeat, owner-fencing, retry + backoff (`attempts`), reclaim (`reclaims`), cancel, poison-cap.
- „Job" mizí jako samostatný koncept → „task, který necheckpointuješ". Atomický „handler-write + complete" nahradí **idempotence** (kterou by stejně bylo potřeba).

**Migrace (náčrt):** `jobs` tabulka + `JobDispatcher` + `worker.Worker`/`runWithinTx` → sjednotit na `runs`/`RunDispatcher`/`RunWorker` (přejmenovat na `task`?). `JobDispatcherMiddleware` → odstranit/sloučit. CLI `worker` co-run jen jednoho workeru. Přepsat `gk-jobs` (nebo sloučit do `gk-runs`), `job-flow.md`, matice.

**Mimo rozsah:** PR #21 (jde první). Tenhle brief je až po jeho merge.


## Rozpad na issues

- [ ] Návrh sjednoceného API (`task` kontrakt, volitelný checkpoint, timeout) + ADR.
- [ ] `durable task` worker = run worker + timeout + bez-checkpoint mód.
- [ ] Migrace `jobs` → `runs` (nebo sjednocené `tasks`); dispatcher + middleware + CLI.
- [ ] Deprecate/odstranit job mechanismus; přepsat docs + skills (`gk-jobs` → `gk-runs`/`gk-tasks`, `job-flow.md`, matice).
- [ ] Ověřit niku „async + DB-only + atomický" — jestli něco v appce takhle běží.


## Související

- [Background work](/framework/background-work) — matice + pravidlo „nevolej ven v transakci".
- [Run flow](/framework/run-flow) · [Job flow](/framework/job-flow).
- Roadmap fáze: [Roadmap](/roadmap) (nahrazuje krok „Konfigurovatelný job lease + heartbeat").
- ADR-0001 — durable execution model.
