---
layout: 'page'
uri: '/framework/gokick-roadmap'
position: 5
slug: 'framework-gokick-roadmap'
parent: 'framework'
navTitle: 'Roadmap (GoKick)'
title: 'Roadmap (GoKick)'
description: 'Forward-looking přehled: co je v gokicku hotové (stručně, po kategoriích) a co zbývá. Jediná rozpracovaná priorita je OpenTelemetry observabilita — detail na konci.'
---

# Roadmap (GoKick)

gokick je **production-ready end-to-end** boilerplate — DDD/CQRS backend, Vue 3 SPA a single-binary deploy. Tenhle dokument je forward-looking: stručně **co je hotové**, pak **co zbývá**. Jediná rozpracovaná priorita je **OpenTelemetry** (detail úplně na konci).


## ✅ Hotovo

**Jádro & architektura**

- DDD 4-vrstvá + CQRS (command/query/event bus s middleware chain), závislosti vrstev vynucené `go-arch-lint`.
- Single-binary deploy — Go binárka s embedovaným Vue SPA i SQL migracemi; migrace se aplikují při startu.
- Wire compile-time DI; perzistence za doménovými `Repository` interface (výměna adapteru bez zásahu do domény).

**Autentizace & bezpečnost**

- JWT access + refresh (HttpOnly cookie) s rotací a detekcí krádeže tokenu.
- Brute-force account lockout + timing-safe login; per-IP rate limiting na auth endpointech.
- CSRF (Go 1.25 stdlib) + kompletní security headers (CSP/HSTS/…); append-only audit log přežívající rollback business transakce.

**Multitenancy**

- Zapínatelný row-level multitenancy (`APP_MULTITENANCY`), fail-closed enforcement, per-dotaz conformance gate v CI.
- Platformní rovina (role `superadmin`, cross-tenant přehled + správa) + system command bus pro CLI (atomicky, s auditem). Detail: `/gk-multitenancy`, `/gk-bus`.

**Background práce**

- In-process scheduler (cron-like goroutiny, panic recovery per-tick).
- Perzistentní durable-task engine (fire-and-forget + durable run, mimo tx, at-least-once, lease/heartbeat/checkpoint) — přežije restart i crash. Detail: `/gk-runs`.

**Frontend**

- Vue 3 SPA (Vite, TypeScript, Tailwind) embedovaná do binárky.
- DataGrid — stránkované/filtrované/řazené admin i platform gridy s bulk akcemi.
- Typovaný fetch — body i response vynuceny generovaným kontraktem (nezkompiluje se bez typu).

**Observabilita**

- Strukturované `slog` logování, jediná cesta staticky vynucená lintem, korelace přes `trace_id`/`user_id`.
- Sentry BE i FE (paniky, terminálně padlé runy, Vue chyby), maskování secretů, light FE↔BE trace linking.

**Tooling & DX**

- BE↔FE typová parita staticky vynucená (tsgen codegen + boundary analyzer + errfields + role uniony + no-as ratchet).
- Go i FE lint ráčna (funlen/gocognit/nestif; max-lines/max-depth/kognitivní komplexita/knip).
- Trunk-based workflow: Conventional Commits + release-please + rebase-only merge + branch ruleset + docpaths gate. Detail: `/gk-deploy`, `CONTRIBUTING.md`.


## 🎯 Kam dál

### 🔴 Škálovatelnost

Největší (a jediný zásadní) strop: single-node SQLite (single-writer) + scheduler bez leader election. SQLite je vědomá volba — řeší se **výměnou adapteru** za doménovými `Repository` interface, ne přepisem aplikace.

- Postgres adaptér — write-scale + RLS jako transparentní-enforcement endgame pro multitenancy.
- Durable fronta na Postgresu → **River** (`SELECT … FOR UPDATE SKIP LOCKED`); scheduler → leader election (advisory locks); rate-limit stav → Redis (stateless instance).
- Alternativa bez Postgresu: distribuovaný SQLite (Turso/libSQL, rqlite, dqlite).

### 🟡 Testovací pokrytí

- E2E (Playwright) — chybí browser flow testy.
- Load testy (k6 / vegeta) — žádné výkonové stropy nejsou ověřené.
- Coverage gate v CI + contract a mutation testy.

### 🟢 Bezpečnost

- 2FA (TOTP) + WebAuthn.
- Refresh token na 256-bit; EdDSA (asymetrický) JWT pro multi-service nasazení.
- CSP bez `unsafe-inline` (nonce-based); gosec + CodeQL + govulncheck v CI; HIBP breach-check hesel.

### 🟢 Architektura

- Zapojit dnes prázdné event/run handler registry reálnými handlery (vzorové příklady).
- Druhý bounded context jako referenční vzor (dnes je doména hlavně auth + users).

### 🟢 Výkon

- Keyset stránkování místo `LIMIT/OFFSET` — list dotazy (`FindPage`, `FindPageAcrossTenants`, `OverviewPageAcrossTenants`) dnes stránkují offsetem: hluboká stránka skenuje a zahazuje všechny předchozí řádky, a souběžný zápis může řádek na hranici stránky duplikovat nebo úplně vynechat.
- Cache vrstva (in-proc / Redis) pro hot reads.
- Benchmarky + pprof profily.

### 🟢 Frontend

- E2E (Playwright), a11y audit (axe), i18n.

### 🟢 Tooling / DX

- Coverage gate + mutation testing v CI.
- Dependabot / Renovate + govulncheck.
- SBOM + podepsané release (cosign) + pre-commit hooky.


## 🔭 OpenTelemetry — rozpracovaná priorita

Jediná nedokončená část observability (proto se drží naživu i throwaway E2E prostředí). Cíl: distribuované tracing napříč FE → HTTP → bus → durable task → SQL, korelované s logy.

- [ ] **OTel HTTP middleware + propagace přes bus** — `trace_id` z ctx přejde na `trace.SpanContext`, sladit se `shared.LogKeyTraceID` (traces ↔ logy korelují).
- [ ] **Span per durable task** (run worker, `process()`) — handler běží outside-tx, takže span obaluje běh handleru, ne transakci; checkpointy/heartbeaty jako child-spany nebo span-events. Plus **SQL viditelnost přes `otelsql`** (span per dotaz). Worker dnes `trace_id` nemá (logy korelují přes `run_id`); OTEL ho nasadí přes `shared.LogAttrs(ctx)`.
- [ ] **FE↔BE distributed tracing — full** — light verze hotová; full přidá `tracesSampleRate > 0` → spany + waterfall (FE klik → API → handler → DB).
- [ ] **Hardening** — `otelsql` + OTEL SDK do depguard allow-listu (`.golangci.yml`); collector endpoint do CSP `connect-src` + `traceparent` přes CORS.
