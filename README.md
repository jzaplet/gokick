---
layout: 'page'
uri: '/'
slug: 'home'
navTitle: '🚀 Getting Started'
title: 'GO Kick'
description: 'Go Kick je produkční základ, na kterém postavíš vlastní aplikaci — ne hotový uzavřený produkt, ale promyšlený startovací bod. Stojí na Go backendu s DDD a CQRS (Command Query Responsibility Segregation), Vue 3 SPA, SQLite databází a JWT autentizací.'
---

# GO Kick

![GO Kick](./docs/go-vue-cqrs-ddd.png "GO Kick")


## 📊 Hodnocení stacku — 8,9 / 10, prakticky best-in-class

> **[⬇ Stáhnout PDF report](./docs/gokick-hodnoceni.pdf)** — nezávislý audit reálného kódu (ne dokumentace) napříč **8 dimenzemi** (bezpečnost, architektura, výkon, škálovatelnost, frontend, tooling, dokumentace & AI skills, testy), se srovnáním s Rails / Laravel / Spring / NestJS a konkrétní cestou k 10/10.


## Proč Go Kick

Bezpečnost i architektura jsou **nad úrovní defaultů velkých frameworků** (Rails, Laravel, NestJS): rotace refresh tokenů s detekcí krádeže, atomický account lockout, timing-safe login, kompletní CSP/HSTS, audit přežívající rollback, reálné DDD + CQRS staticky vynucené `go-arch-lint` a jediná logovací cesta hlídaná lintem.

Umí i **zapínatelný row-level multitenancy** — jedním přepínačem (`APP_MULTITENANCY`), bez jakéhokoli dopadu na single-tenant nasazení.

Běží bez Redisu, brokeru či jiné externí infrastruktury a nasazuje se jako jedna samostatná binárka (s embedovaným frontendem i migracemi). Jediný vědomý strop — single-node SQLite bez horizontálního škálování — z něj dělá ideální volbu pro malá až střední nasazení s důrazem na bezpečnost, čistotu a jednoduchý provoz.


## Vyzkoušej a prozkoumej

- Aplikace: <https://gokick-app.strategio.dev> — běží v multitenant režimu, tři showcase účty (všechny v tenantu `Default`):
    - **superadmin** / `Superadmin` — platformní rovina: cross-tenant přehled + správa tenantů a uživatelů
    - **admin** / `Admin1234` — správa uživatelů ve svém tenantu
    - **User** / `User1234` — běžný uživatel
- Dokumentace: <https://gokick.strategio.dev/>
- GitHub: <https://github.com/jzaplet/gokick>

## Vlastnosti

- **DDD** – čtyřvrstvá architektura (domain → application → infrastructure → presentation) s bounded kontexty, entitami, value objects a domain eventy
- **CQRS** – oddělené command/query/event busy s middleware chain (logging, autorizace, transakce, recovery)
- **Dependency inversion** – doména definuje interfaces (porty), infrastruktura dodává implementace (adaptery). Př: SQLite lze zaměnit za Postgres bez zásahu do domény
- **Multitenancy na přepínač** – zapínatelný row-level multitenancy (`APP_MULTITENANCY`, default vypnuto = single-tenant) s izolací vynucenou per-dotaz conformance testem; platformní rovina (role `superadmin`, cross-tenant přehled + správa) a CLI pro správu tenantů
- **Vue 3** SPA (Vite, TypeScript, Tailwind) embedovaná do Go binárky
- **SQLite** s migracemi (Goose), pure-Go bez CGO
- **JWT** access + refresh token autentizace
- **Wire** compile-time dependency injection
- **go-arch-lint** vynucení závislostí mezi vrstvami
- **Sentry** – error tracking BE i FE (paniky, terminálně padlé background úlohy, Vue chyby), gated na DSN; maskování credential hlaviček + FE↔BE trace linking
- **Strukturované logování** – `slog` s konstantními klíči a korelací přes `trace_id`/`user_id`, jediná logovací cesta staticky vynucená lintem (depguard/forbidigo/sloglint)
- **Audit log** – append-only záznam security-relevantních akcí (login failed, account locked, theft detected, role changed); persistuje i při rollbacku business transakce
- **Rate limiting** – per-IP token bucket na `/auth/login` (default 10/min) a `/auth/refresh` (60/min), konfigurovatelné přes `.env`
- **Brute-force ochrana** – zámek účtu po 5 selháních / 10 min na 15 min; přihlášení běží v konstantním čase (neprozradí existenci uživatele ani stav zámku)
- **CSRF** – `http.CrossOriginProtection` (Go 1.25 stdlib) přes `Sec-Fetch-Site`, plus `SameSite=Strict` na refresh cookie
- **Security headers** – CSP, HSTS (gated na HTTPS), `X-Frame-Options: DENY`, Permissions-Policy, COOP/CORP — cíl A+ na securityheaders.com
- **In-process scheduler** – cron-like periodické úlohy (goroutiny + ticker, run-once-then-tick, panic recovery per-tick); první uživatel: cleanup expirovaných refresh tokenů
- **Perzistentní durable engine** (fire-and-forget + durable run) – SQLite-backed background work s durable-task workerem (at-least-once, atomický claim, exponenciální backoff); přežije restart i crash procesu


## Rychlý start

1. **Naklonuj repo** (nebo si založ vlastní repo z template přes „Use this template").
2. V Claude Code napiš **`/gk-init`** — provede tě od nuly k běžícímu serveru (install, build, seed admina, dev loop).

Na vše ostatní stačí **`/gk`** — rozcestník, který sám rozhodne, jaké další `gk-*` skills na tvůj úkol použít. Manuální kroky najdeš v [Installation](/framework/installation).


## Hlavní příkazy

| Příkaz | Co dělá |
|---|---|
| `make build` | Sestaví frontend + backend → `bin/app` |
| `make serve` | Spustí server |
| `make test` | Vitest + go test |
| `make lint` | ESLint + vue-tsc + knip + golangci-lint + go-arch-lint + golines format-check + ts-check + documan-lint |
| `make format` | ESLint Stylistic + golines + documan-fix |


## Dokumentace

| Sekce | Popis                                                |
|-------|------------------------------------------------------|
| [AI skills](/skills) | **gk-* skills** — přesné koncepty + how-to pro AI i vývojáře (napiš `/gk`) |
| [Framework](/framework) | Architektura, instalace, konfigurace + toky (HTTP, CQRS, Background) |
| [ADRs](/adrs) | Architecture Decision Records — zafixovaná rozhodnutí **(template pro tvůj projekt)** |
| [Roadmap](/roadmap) | Fázovaný plán **(template pro tvůj projekt)** |
| [Issues](/issues) | Features / Bugs / Chores **(template pro tvůj projekt)** |
| [Briefs](/briefs) | Rozvahy za rozhodnutími **(template pro tvůj projekt)** |

Detailní „jak se co dělá" žije v **`gk-*` skillech** (`.claude/skills/`) — po naklonování napiš `/gk`. Dokumentace je tenký rozcestník + core stránky.
