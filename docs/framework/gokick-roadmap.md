---
layout: 'page'
uri: '/framework/gokick-roadmap'
position: 5
slug: 'framework-gokick-roadmap'
parent: 'framework'
navTitle: 'Roadmap (GoKick)'
title: 'Roadmap (GoKick)'
description: 'Aktuální priorita (OpenTelemetry) + bodová cesta k 10/10 v každé disciplíně. Plné hodnocení stacku v PDF reportu.'
---

# Roadmap (GoKick)


## 📊 Hodnocení stacku — 8,5 / 10

> **[⬇ Stáhnout PDF report](../gokick-hodnoceni.pdf)** — nezávislý audit reálného kódu.

<a href="../gokick-hodnoceni.pdf"><img src="../go-vue-cqrs-ddd.png" alt="Hodnocení stacku gokick — PDF report" width="200"></a>

Boilerplate je **production-ready end-to-end**: DDD/CQRS backend, Vue 3 SPA, JWT auth s HttpOnly refresh cookie a detekcí krádeže, admin user CRUD, perzistentní job queue + scheduler, rate limiting, audit log, brute-force lock, security headers, Sentry (BE i FE), single-binary deploy. Fáze 1–4 a většina fáze 5 jsou hotové (rekapitulace v sekci **Hotovo** níže).

Tenhle dokument je **forward-looking**: co zbývá jako aktuální priorita a co konkrétně chybí do plné desítky v každé disciplíně.


## Aktuální priorita — OpenTelemetry

Jediná otevřená položka původního fázového plánu. Má smysl až aplikace poběží v produkci vedle alespoň jedné další služby — pro standalone monolit přidává složitost bez návratnosti, proto **volitelně, později**.

- [ ] **OTel HTTP middleware + propagace přes bus middleware** — `traceID` v contextu může přejít na `trace.SpanContext`.
- [ ] **Tracing job workeru** — span per job s `kind` a `attempts` jako atributy.
- [ ] **SQL viditelnost přes `otelsql`** (rozhodnuto 2026-06-15) — obalí DB driver → span per dotaz (text + trvání), zviditelněno v traces i na Sentry erroru. Proto se vědomě **nestaví** vlastní SQL → breadcrumb most — OTel ho nahradí.
- [ ] **FE↔BE distributed tracing — full (rozsah B)** — light verze hotová ([PR #11](https://github.com/jzaplet/gokick/pull/11)): FE i BE sdílí Sentry trace id přes `sentry-trace`/`baggage`. Full přidá `tracesSampleRate > 0` → spany + waterfall (FE klik → API span → BE handler → DB).
- [ ] **BE source context u Sentry framu** (volitelné, mimo OTel) — Sentry GitHub integrace (code mapping), ne posílání zdrojáku do image. Nízká priorita.


## Cesta k 10/10

Co konkrétně chybí do plného skóre v jednotlivých disciplínách. **Jedna disciplína nese ~90 % práce** — škálovatelnost; zbytek představují cílené dílčí úpravy v řádu týdnů. Dokumentace & AI skills je na **10/10** (ADRs jsou součástí šablony, ne tohoto projektu — do hodnocení se nepromítají). Detaily a kontext v [PDF reportu](../gokick-hodnoceni.pdf), kapitola 9.

### 🔴 Škálovatelnost `4 → 10` — sem patří většina práce

Největší (a jediný zásadní) strop: single-node SQLite (single-writer) + scheduler bez leader election. **SQLite je přitom vědomá volba, ne nedopatření** — řeší se adaptérem, ne opuštěním návrhu. A protože perzistence sedí za doménovými `Repository` interface, jde o **výměnu adapteru, ne přepis aplikace**. Preferovaná cesta je **adaptér na Postgres**; alternativně lze zůstat u distribuovaného SQLite. Dvě cesty:

- **A) Zůstat u SQLite (HA + read-scale):**
  - **Turso / libSQL** — embedded replicas + automatický sync, nově i concurrent writes (MVCC, obchází single-writer bottleneck).
  - **rqlite** — Raft, nejzralejší clustering; **dqlite** (Canonical, Raft).
  - LiteFS funguje, ale je pre-1.0 a Fly.io ho deprioritizoval → pro nové projekty spíš Turso.
- **B) Skutečný write-scale (Postgres):**
  - Přidat `infrastructure/postgres/*` + `wire.Bind` na stávající doménové interface (adapter swap).
  - Job frontu nahradit **River** (Postgres-native, battle-tested) místo custom SQLite queue.
  - Scheduler ošetřit **leader election** (Postgres advisory locks) — konec double-runů na víc instancích.
  - Rate-limit stav externalizovat (Redis), aby instance byly stateless.

### 🟡 Testovací pokrytí `7,5 → 10`

- **E2E** (Playwright) — chybí browser flow testy.
- **Load testy** (k6 / vegeta) — žádné výkonové stropy nejsou ověřené.
- **Coverage gate** v CI + **contract** a **mutation** testy.

### 🟢 Bezpečnost `9 → 10`

- **2FA** (TOTP) + **WebAuthn**.
- Refresh token na 256-bit; **EdDSA** (asymetrický) JWT pro multi-service nasazení.
- CSP bez `unsafe-inline` (nonce-based); **gosec + CodeQL + govulncheck** v CI; HIBP breach-check hesel.

### 🟢 Architektura `9 → 10`

- Zapojit dnes prázdné **event/job handler registry** reálnými handlery (vzorové příklady).
- Druhý **bounded context** jako referenční vzor (dnes je doména hlavně auth + users).

### 🟢 Výkon `8,5 → 10`

- **Keyset stránkování** všech list dotazů (dnes `FindAll` tahá celou tabulku).
- **Cache vrstva** (in-proc / Redis) pro hot reads.
- **Benchmarky + pprof** profily.

### 🟢 Frontend `8,5 → 10`

- **E2E** (Playwright), **a11y** audit (axe), **i18n**.

### 🟢 Tooling / DX `9,5 → 10`

- **Coverage gate + mutation testing** v CI.
- **Dependabot / Renovate** + govulncheck.
- **SBOM** + podepsané release (cosign) + pre-commit hooky.

## Hotovo (F1–F5)

Rekapitulace — detailní záznam (Definition of Done, regresní testy, klíčová rozhodnutí) je v git historii tohoto souboru.

- **F1 — Event flow & graceful shutdown** (2026-05-17) — request-scoped `EventCollector` (konec race), DispatchEvents přesunut ven z transakce (dispatch až po commitu), SIGTERM drain.
- **F2 — In-process scheduler** (2026-05-17) — cron-like goroutiny s tickerem, run-once-then-tick, panic recovery per-tick; první job: cleanup expirovaných refresh tokenů.
- **F3 — Perzistentní job queue (SQLite)** (2026-05-17) — atomický claim přes `UPDATE … RETURNING`, exponenciální backoff, at-least-once, mark-complete v handler tx, worker pool.
- **F4 — Hardening** (2026-05-17) — 3 kritické fixy z auditu + rate limiting, brute-force lock, audit log mimo transakci, HTTP boundary hardening, SQLite concurrency fix (`_txlock=immediate`).
- **F5 — Observability** — strukturované slog atributy se statickým lint-enforcementem + Sentry BE/FE s obohacením eventu a maskováním tajemství (2026-06-10 / 06-14). Zbývá už jen OTel (viz **Aktuální priorita** výše).
