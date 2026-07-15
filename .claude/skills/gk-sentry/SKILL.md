---
layout: 'page'
uri: '/skills/gk-sentry'
position: 20
slug: 'skills-gk-sentry'
parent: 'skills-observability'
navTitle: 'gk-sentry'
title: 'GK — Sentry error tracking'
description: 'Error tracking (Sentry) na BE i FE — hlásí jen neočekávaná selhání (paniky, terminálně padlé runy, Vue chyby), obohacuje event o uživatele/request/breadcrumbs, je vypnuté bez DSN. Use when zapínáš Sentry, řešíš proč nepřišel/přišel event, nastavuješ FE DSN nebo source mapy, nebo nevíš, co se vlastně reportuje.'
name: 'gk-sentry'
---

# GK — Sentry error tracking

Hlášení **neočekávaných** selhání do Sentry — backend i frontend. Bez DSN je celé no-op: aplikace běží stejně a neposílá nic.

## What & when

- Sáhni sem, když: **zapínáš Sentry** (zakládáš projekty, nastavuješ DSN), řešíš **proč event nepřišel** (typicky FE „tiše neaktivní"), chceš vědět **co přesně se reportuje** a jak je event obohacený, nebo nastavuješ **FE source mapy**.
- NEtýká se běžných chyb: validace, auth, 4xx se do Sentry **nikdy** nehlásí — ty jdou přes doménové chyby → HTTP status (viz `/gk-errors`). Sentry není logovací cesta — logování řeší `/gk-logging`. Maskování tajemství před odchodem do Sentry je `/gk-hardening`.
- Performance tracing (spany, waterfall) a OpenTelemetry **nejsou hotové** — jsou na roadmapě. Tenhle skill pokrývá jen chyby a paniky (rozsah A).

## For non-tech / juniors

Sentry je „černá skříňka" pro chyby: když aplikace spadne způsobem, který nikdo nečekal (program se zhroutí — *panika* na BE, nezachycená výjimka na FE), pošle se popis chyby + kontext na vzdálený dashboard, kde to vývojář vidí seskupené a s detaily. Cíl: dozvědět se o problému dřív, než ho nahlásí uživatel.

Klíčové: **hlásí se jen to neočekávané.** Když uživatel zadá špatné heslo, to není chyba aplikace — to je normální stav (vrátí se 401) a do Sentry to nejde. Kdyby tam šlo všechno, tracker se zahltí šumem a opravdové pády se ztratí.

A bez „adresy" (DSN — veřejný odkaz na tvůj Sentry projekt) je všechno vypnuté. Můžeš tedy gokick provozovat úplně bez Sentry účtu a zapnout ho až potřebuješ.

## How it works

**Port `shared.ErrorReporter`** (`app/domain/shared/error_reporter.go`) — doménové rozhraní se 4 metodami: `Capture(ctx, err, attrs…)`, `WithRequestScope(ctx)`, `ContinueTrace(ctx, sentryTrace, baggage)`, `Flush(timeout)`. Implementace je v `cmd/sentry.go` (vedle loggeru) a **injektuje se přes DI**, takže `sentry-go` import zůstává mimo vrstvený `app/` strom. Prázdné DSN → `NopReporter{}` (no-op).

**Kdo volá `Capture` (jen 3 místa — recovery + terminal):**
- bus `RecoveryMiddleware` (`app/application/bus/middleware/recovery.go`) — panika v command/query handleru,
- HTTP `RecoveryMiddleware` (`app/presentation/http/middleware/recovery.go`) — panika v requestu → log + report + 500,
- run worker (`app/infrastructure/worker/run_worker_process.go`) — run s **vyčerpanými retries** (terminálně padlý), neznámý kind, poison run (překročený limit reclaims) nebo panika v procesu/heartbeatu workeru. — A v Invariants bullet doplnit: „Hlásí se jen recovery (bus + HTTP `RecoveryMiddleware`, paniky ve workeru) a terminálně padlé runy (worker)."

Běžné návratové chyby se nehlásí — proto je `Capture` jen na těchto cestách.

**Obohacení eventu** (`cmd/sentry.go`, `Capture`):
- **User** — `id` / `nickname` / `role` / `email` z `AuthClaims` v ctx + rozlišená klientská IP (`sentryUser`). Vyžaduje `SendDefaultPII: true` v `Init`, jinak sentry-go ručně nastavenou IP/email zahodí.
- **Request** — `method` / `url` / `User-Agent` z předaných attrs (`sentryRequest`); jen u HTTP callera (worker nemá `method` → bez requestu).
- **Tags** — ostatní attrs jako vyhledatelné tagy; u paniky navíc typ exception `panic` + tag `panic.type`, u workeru fingerprint `["{{ default }}", "run:<kind>"]` (každý padající handler vlastní issue).
- **Maskování** — credential hodnoty se redaktují na **každém** egress bodu (hlavičky, tagy, breadcrumbs), ne whitelistem. Detail v `cmd/sentry.go` + `app/domain/shared/mask.go`.

**Breadcrumbs** = stopa `INFO+` log řádků vedoucích k chybě (jako Symfony Monolog trail). `ReportScopeMiddleware` (`app/presentation/http/middleware/reportscope.go`, zapojený **před** Recovery) zavolá `WithRequestScope` a naváže per-request Sentry hub na ctx; `breadcrumbHandler` v `cmd/logger.go` (obal slog handleru) přidá každý `INFO+` záznam na ten hub. **Jen ctx-form logy** (`LogAttrs(ctx, …)`) breadcrumb nesou.

**FE↔BE trace linking (light)** — FE i BE sdílí jeden Sentry *trace id*, takže FE chyba a BE chyba, kterou způsobila, jsou v Sentry **propojené pod jedním trace**. FE (`initSentry.ts`) má `browserTracingIntegration` + `tracesSampleRate: 0`: propaguje `sentry-trace`/`baggage` na same-origin `/api` volání, ale **žádná performance data neposílá** (transakce sampled 0). BE (`ReportScopeMiddleware` → `ContinueTrace`) trace id přebere a ručně nasadí na scope (`EnableTracing` zůstává false). **Rozsah:** linkne se FE *chyba* ↔ BE *chyba*; běžné „FE akce → BE 500" na FE žádný event nevytvoří (handled 5xx se nehlásí), takže ten trace nese jen BE stranu. Pozn.: gokick `trace_id` z `TraceMiddleware` je **interní korelace BE logů**, ne tenhle Sentry trace.

**Frontend** (`assets/app-ui/Sentry/`): `initSentry` (volaný v `app.ts` před prvním `await`) zachytává Vue chyby + unhandled promise rejections; handled API 4xx z `authFetch`/`apiFetch` se **nehlásí**. Browser SDK sbírá breadcrumbs (kliky, navigace) automaticky. `syncUser.ts` drží `Sentry.setUser` v zámku se session přes `watch` nad jediným `user` ref — login/refresh/logout ho aktualizují, žádná auth cesta nezapomene.

**Lifecycle:** `Init` při startu, `defer reporter.Flush(...)` v `run()` v `cmd/main.go` — `main()` je jen `os.Exit(run())`, takže defer proběhne na každé exit cestě (žádný `os.Exit` ho nepřeskočí) — `CaptureException` je async, jinak by panika/exit event ztratily.

## Recipe: zapnout Sentry

1. **Založ dva projekty** — BE (platforma Go) a FE (Vue/Browser) jsou samostatné projekty → **dva DSN**. DSN je **veřejná** hodnota (ne tajemství), klidně ji commitni do deploy configu.
2. **Backend** — runtime env binárky: `APP_SENTRY_DSN=...` + `APP_SENTRY_ENVIRONMENT=production`. (Čte je `cmd/` ještě před `LoadConfig` přes `StartupConfig`. `APP_SENTRY_DSN` je jen v `StartupConfig`, ne v `Config` struct; `APP_SENTRY_ENVIRONMENT` je v obou.)
3. **Frontend — pozor na past:**
   - **Prod** (Docker image): nastav `APP_SENTRY_DSN_FRONTEND` jako **runtime env kontejneru**. Go server ho injektuje do `index.html` jako `<meta name="gokick:sentry-dsn">` a SPA ho čte přes `runtimeConfig.ts`. → „build once, deploy many".
   - **Dev** (Vite dev server): build-time `VITE_SENTRY_DSN` v `.env`.
   - **NIKDY nenastavuj `VITE_SENTRY_DSN` v produkci** — FE Sentry by zůstal **tiše neaktivní** (přesně tahle past). V produkci jen `APP_SENTRY_DSN_FRONTEND`.
4. **CSP** se řeší samo — `SecurityHeadersMiddleware` přidá ingest origin FE DSN do `connect-src`, jakmile je DSN nastavený.
5. **Za Cloudflare**: `APP_TRUST_PROXY_HEADERS=true`, aby `user.ip_address` byla skutečná IP (jen za origin-lock firewallem — hlavičky jsou jinak podvrhnutelné).
6. **Ověř** přes `APP_SENTRY_DEBUG=true` (BE i FE): `curl https://<host>/debug/sentry` → 500 + BE event; v aplikaci se zobrazí červené tlačítko (`SentryDebugTrigger.vue`) → klik → FE event. Pak `APP_SENTRY_DEBUG` **vypni** (aplikace při startu varuje, dokud je zapnutý).

## Recipe: FE source maps (opt-in)

Bez nich jsou FE stack traces minifikované. `@sentry/vite-plugin` mapy nahraje při buildu a hned je smaže z dist (`public/` se embeduje do binárky → žádná `.map` nesmí zůstat). Zapneš třemi build-time hodnotami v CI:

1. `SENTRY_AUTH_TOKEN` — **tajný** token (GitHub repo *secret*): `gh secret set SENTRY_AUTH_TOKEN`.
2. `SENTRY_ORG`, `SENTRY_PROJECT` (FE projekt) — ne-tajné (GitHub repo *vars*): `gh variable set ...`.
3. Cut tag `vX.Y.Z` → release workflow mapy nahraje. Špatný token = build padne červeně (žádný tichý průchod).

## Invariants & pitfalls

- **Sentry NENÍ logovací cesta.** Hlásí se jen recovery (bus + HTTP `RecoveryMiddleware`) a terminálně padlé runy (worker). Validace / auth / 4xx **nikdy** — jinak tracker utone v šumu. Pro chyby s HTTP statusem viz `/gk-errors`.
- **Nikdy nepiš ruční `reporter.Capture` do command/query handleru.** Reporting je jen na recovery/terminal cestách; handler vrací chybu, mapování řeší vrstva výš.
- **Bez DSN = no-op.** BE → `NopReporter`, FE → `initSentry` se hned vrátí. Aplikace běží beze změny.
- **`sentry-go` je jediný depguard-allow-listed non-slog sink.** Import smí být **jen v `cmd/`**, ne ve vrstveném `app/` stromu — proto je reporter doménový port (viz `/gk-architecture`).
- **DSN je veřejné** (klidně v HTML/configu); **`SENTRY_AUTH_TOKEN` je tajný** (jen CI secret, po použití revokuj).
- **`APP_SENTRY_DEBUG` nikdy v produkci** — aplikace při startu varuje, dokud je zapnutý.
- **`VITE_SENTRY_DSN` v prod image = FE Sentry neaktivní.** V produkci jedině runtime `APP_SENTRY_DSN_FRONTEND`.
- **Breadcrumb nesou jen ctx-form logy** (`LogAttrs(ctx, …)`) — log bez ctx se na hub nedostane, takže ve stopě k chybě chybí.

## Related

- `/gk-errors` — kam jdou validace/auth/4xx (kontrast: ty se NEreportují).
- `/gk-runs` — durable worker a terminal-failure reporting (vyčerpané retries → `Capture`).
- `/gk-bus` — `RecoveryMiddleware` v middleware chainu.
- `/gk-logging` — strukturované logy, ze kterých se skládají breadcrumbs (`LogAttrs(ctx, …)`).
- `/gk-hardening` — maskování credential hlaviček/hodnot před odchodem do trackeru.
- `/gk-config` — env proměnné (`APP_SENTRY_*`, `APP_TRUST_PROXY_HEADERS`).
- `/gk-architecture` — proč `sentry-go` zůstává v `cmd/` a reporter je port.
- Kód: `cmd/sentry.go` (adaptér), `cmd/logger.go` (`breadcrumbHandler`), `app/domain/shared/error_reporter.go` (port), `app/presentation/http/middleware/reportscope.go`, `assets/app-ui/Sentry/` (FE).
