---
layout: 'page'
uri: '/framework/gokick-roadmap'
position: 5
slug: 'framework-gokick-roadmap'
parent: 'framework'
navTitle: 'Roadmap (GoKick)'
title: 'Roadmap (GoKick)'
description: 'Aktuální priorita F6 — zapínatelný row-level multitenancy + OpenTelemetry observabilita; bodová cesta k 10/10 v každé disciplíně. Plné hodnocení stacku v PDF reportu.'
---

# Roadmap (GoKick)


## 📊 Hodnocení stacku — 8,5 / 10

> **[⬇ Stáhnout PDF report](../gokick-hodnoceni.pdf)** — nezávislý audit reálného kódu.

<a href="../gokick-hodnoceni.pdf"><img src="../go-vue-cqrs-ddd.png" alt="Hodnocení stacku gokick — PDF report" width="200"></a>

Boilerplate je **production-ready end-to-end**: DDD/CQRS backend, Vue 3 SPA, JWT auth s HttpOnly refresh cookie a detekcí krádeže, admin user CRUD, perzistentní job queue + scheduler, rate limiting, audit log, brute-force lock, security headers, Sentry (BE i FE), single-binary deploy. Fáze 1–4 a většina fáze 5 jsou hotové (rekapitulace v sekci **Hotovo** níže).

Tenhle dokument je **forward-looking**: co zbývá jako aktuální priorita a co konkrétně chybí do plné desítky v každé disciplíně.


## 🚀 Aktuální priorita — Multitenancy + observabilita (F6)

gokick dostává **zapínatelný row-level multitenancy** (jedním přepínačem; vypnuto = dnešní chování) a **OpenTelemetry**. Obojí je široce přenositelné do dalších projektů — chceme to mít připravené v jádře, ne dodělávat per-projekt a pak portovat zpět. Pořadí je **závazné**: kroky 1–4 jsou multitenant páteř (odladitelná na jednom default tenantu), krok 5 běží paralelně, krok 6 (OTEL) až na finálním tvaru stroje, krok 7 (Postgres) je odložený.

**Proč tenant a ne `user_id`.** `user_id` je „kdo jsi", `tenant_id` je „čí jsou data" — dnes splývají 1:1, ale scopovat podle `user_id` natvrdo říká „navždy 1 user = 1 dataset". Tenant je hranice vlastnictví: unese týmy, převod vlastnictví, per-workspace billing/kvóty i service accounts. Hlavně: `tenant_id ≈ user_id` teď je no-op, kdežto `user_id → tenant_id` potom je backfill + přepis všech dotazů pod zátěží. Výchozí cesta přepínače: **auto 1 tenant per user** při signupu (owner = user), nula UX rozdílu dnes.

**Izolační model — row-level (rozhodnuto), ne DB-per-tenant.** Bez ORM neexistuje global scope zadarmo: `Conn(ctx)` vykonává hotové SQL stringy, takže **nedokáže transparentně vstříknout `WHERE tenant_id`**. Resolver tenant jen **dodá**, repo ho **aplikuje**. Záruka „nerozbijeme to při vývoji" stojí na dvou vrstvách, které se kryjí:

- **`tenant_id NOT NULL` + FK** — chybějící stamp na INSERTu spadne hlasitě za běhu.
- **`zz_tenant_test.go` conformance gate** — projde repo SQL, spadne v CI, když dotaz nad tenant-owned tabulkou nemá `tenant_id` (styl stávajících `zz_audit`/`zz_gap` testů).

⚠️ **Hlavní riziko SQLite fáze, pojmenované poctivě:** záruka je **asymetrická** — chybějící WHERE na *čtení* leakuje **tiše** a chytí ho **jen CI**. Slepé místo scanneru (dynamicky stavěné SQL, dotazy mimo skenované adresáře, JOIN) = cross-tenant read se vyveze. **Transparentní vynucení dá až Postgres RLS (krok 7)**; do té doby resolver predikát jen dodává, nevynucuje.

### Krok 1 — Tenant páteř (nula změn chování)

- [ ] **`AuthClaims.TenantID`** — jedno pole, jede v JWT claimu i ctx (jako dnes `UserID`).
- [ ] **`TenantMiddleware`** hned za `AuthorizeMiddleware` — přečte tenant z claims do ctx.
- [ ] **`TenantResolver` interface** + default **single-tenant no-op** resolver.
- [ ] Mergovatelné samostatně: žádná tabulka zatím sloupec nenese → chování = dnešek.

### Krok 2 — `domain/tenant` + bootstrap + backfill

- [ ] **`domain/tenant/`** — entita + `Repository` + arch-lint **`domain_tenant`** komponenta + `mayDependOn` granty od konzumentů.
- [ ] **Migrace `tenants`** (control-plane) + `users.tenant_id`.
- [ ] **Bootstrap tenant row + backfill** existujících `users.tenant_id` **dříve**, než lze zapnout NOT NULL FK (krok 3).
- [ ] **Seeder + `console/create_user.go`** musí tenant resolvovat/zakládat — jinak po NOT NULL FK selžou.
- [ ] **Auto 1 tenant per user** v `CreateUserHandler` — výchozí cesta přepínače (boilerplate, ne produkt).
- [ ] No-op resolver z kroku 1 vrací **id bootstrap tenanta**, ne prázdno (po kroku 3 každý zápis potřebuje reálné id).

### Krok 3 — Row-level enforcement + worker propagace

- [ ] **`tenant_id NOT NULL` + FK** na owned tabulkách, composite indexy `(tenant_id, …)`.
- [ ] **`BaseRepository.Tenant(ctx)`** helper — v multitenant režimu panika, když tenant chybí (nedovolí nescopovaný dotaz).
- [ ] **`zz_tenant_test.go`** conformance scan SQL.
- [ ] **Worker tenant propagace (nejvyšší hodnota celé fáze):** worker obchází bus → `TenantMiddleware` se na job handlery **nikdy nespustí**. Stampuj `jobs.tenant_id` **při enqueue** (in-bus přes JobDispatcher ctx) a worker v `runWithinTx` **obnoví `tenant_id` do ctx z claimnutého `jobs` řádku** před voláním handleru. `ClaimDue` zůstává **globální drain** (bez tenant filtru — jeden worker obsluhuje všechny tenanty) → documented conformance exempce. **Musí přijít s páteří, ne se objevit potom** — jinak první tenant-owned zápis z jobu = tvrdý NOT NULL break a tiché leaky na čtení.

### Krok 4 — JWT nese tenant

- [ ] `tenant_id` do JWT claimu (mint + verify) a kopie do `AuthClaims`.

### Krok 5 — Konfigurovatelný job lease + heartbeat (paralelní)

- [ ] Nahradit zadrátovaný `defaultLockFor = 5min` **per-kind konfigurovatelným lease** + **heartbeat/renewal**, ať dlouhé joby (agentická práce) neztratí lock uprostřed běhu.
- [ ] **Renewal přes raw pool (`r.DB.DB()`), mimo job tx** — obnova uvnitř `runWithinTx` je pro ostatní workery neviditelná až do commitu, což maří účel. Legitimní documented raw-pool případ (jako audit / failed-login).

### Krok 6 — OpenTelemetry (až na finálním tvaru)

- [ ] **OTel HTTP middleware + propagace přes bus** — `trace_id` v ctx přejde na `trace.SpanContext`; **sladit s `shared.LogKeyTraceID`**, ať traces a logy korelují.
- [ ] **Tracing job workeru** — span per job s `kind` a `attempts` jako atributy.
- [ ] **SQL viditelnost přes `otelsql`** (rozhodnuto 2026-06-15) — obalí DB driver → span per dotaz (text + trvání). Proto se vědomě **nestaví** vlastní SQL → breadcrumb most.
- [ ] **FE↔BE distributed tracing — full (rozsah B)** — light verze hotová ([PR #11](https://github.com/jzaplet/gokick/pull/11)): FE i BE sdílí trace id přes `sentry-trace`/`baggage`. Full přidá `tracesSampleRate > 0` → spany + waterfall (FE klik → API span → BE handler → DB).
- [ ] **Závislosti & hardening:** `otelsql` + OTEL SDK do **depguard allow-listu v `.golangci.yml`** (jinak lint padne); collector endpoint do **CSP `connect-src`** + `traceparent` přes CORS (seam, kde už žije Sentry ingest).
- [ ] **(volitelné, mimo OTel)** BE source context u Sentry framu přes Sentry GitHub integraci (code mapping), ne posílání zdrojáku do image. Nízká priorita.

### Krok 7 — Postgres adapter (odložené)

- [ ] Až podle kalkulace **~1000 klientů**; do té doby jen držet `Conn`/`Transactor`/`TenantResolver` seam čistý (gokick je z 90 % ready návrhem). Navazuje na disciplínu **Škálovatelnost** níže. **Postgres RLS je ten transparentní-enforcement endgame**, který zavře tichý leak na čtení z SQLite fáze.

### Legitimně nescopované — conformance-test exempce

Ne všechno smí nést povinný tenant filtr; tyhle jsou vědomě na allowlistu (jako stávající raw-pool výjimky):

- **`audit_log`** — control-plane, raw pool, zaznamenává **pre-auth** eventy (`auth.login.failed`, `auth.account.locked`), kde tenant neexistuje.
- **`users`** (login/refresh path) — identity-root tabulka, hledá se podle `nickname`/`id` **před** resolucí auth.
- **`refresh_tokens`** — `FindByHash` podle tajemství (refresh běží právě když access token chybí).
- **`ClaimDue`** — globální drain workeru (viz krok 3).


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
- **F5 — Observability** — strukturované slog atributy se statickým lint-enforcementem + Sentry BE/FE s obohacením eventu a maskováním tajemství (2026-06-10 / 06-14). OTel je teď součástí fáze **F6** (viz **Aktuální priorita** výše).
