---
layout: 'page'
uri: '/skills/gk'
position: 15
slug: 'skills-gk'
parent: 'skills'
navTitle: 'gk'
title: 'GK — rozcestník skillů'
description: 'Rozcestník po gokick projektu — přehled všech gk-* skillů a co který řeší, od git clone po deploy a observabilitu. Use when naklonuješ projekt a chceš vědět co všechno s ním můžeš dělat, nebo nevíš, který specializovaný skill na svůj problém sáhnout.'
name: 'gk'
---

# GK — rozcestník skillů

Gokick je Go **DDD/CQRS** skeleton s Vue 3 SPA, SQLite a JWT autentizací — celá
aplikace v jediné binárce. Tahle rodina `gk-*` skillů popisuje **přesné koncepty
a jak se co používá**: každý skill má „proč to použít · na co je vhodné · jak to
funguje · recept · časté chyby" a je psaný i pro juniory / non-tech.

**Jak na to:** napiš `/gk-<název>` (např. `/gk-repositories`).
Nebo prostě popiš problém a AI sáhne po správném skillu sama (řídí se podle
`description` každého skillu).

## Cesta projektem (od initu po observabilitu)

### Start & orientace
| Skill | Co řeší |
|-------|---------|
| `/gk-init` | Od `git clone` **nebo template** k běžícímu + nakonfigurovanému repu — install, build, seed, dev loop, `make setup-github` |
| `/gk-config` | `.env` konfigurace: co se čte, kdy, defaulty, kde se validuje |
| `/gk-architecture` | Mentální model: DDD 4 vrstvy, CQRS, dependency rules, bounded contexts |
| `/gk-feature` | Přidat novou featuru **end-to-end** — checklist napříč všemi vrstvami |

### Doména
| Skill | Co řeší |
|-------|---------|
| `/gk-entities` | Entity (db tagy) + value objects s validací + factory funkce |
| `/gk-domain-events` | „Stalo se X" → reakce po commitu, aniž to command handler zná |
| `/gk-errors` | Doménové chyby (Validation/Auth/Permission) → automaticky HTTP status |
| `/gk-i18n` | Překladové klíče přes API ({key, params}), render na FE (t()/tm()), jediný katalog locale/, jak přidat text |

### Application (CQRS)
| Skill | Co řeší |
|-------|---------|
| `/gk-bus` | Command/Query/Event busy, middleware chain a jeho pořadí, dispatch |
| `/gk-commands` | Write operace — struktura handleru + povinná deklarace permission |
| `/gk-queries` | Read operace — struktura query handleru + typovaný návrat |

### Data & infrastruktura
| Skill | Co řeší |
|-------|---------|
| `/gk-repositories` | Datová vrstva nad SQLite — tx-aware `r.Conn(ctx)`, raw-pool výjimky, tuning |
| `/gk-migrations` | Goose migrace — embedované, automaticky aplikované při startu |
| `/gk-runs` | Durable engine pro background práci mimo tx (přežije restart/crash) — fire-and-forget run (FireAndForget) i dlouhý **run** s checkpoint + resume (Durable) |
| `/gk-scheduler` | Periodické úlohy uvnitř serveru (cron-like, bez OS cronu) |
| `/gk-di` | Wire compile-time DI — providery, `wire.Bind`, `make di` |

### Auth & bezpečnost
| Skill | Co řeší |
|-------|---------|
| `/gk-auth` | JWT access + refresh, rotace + theft detection, login, session, logout |
| `/gk-permissions` | Kdo smí co — BE `Permissioned` + FE enum, jeden zdroj pravdy |
| `/gk-multitenancy` | Zapínatelný row-level multitenancy + platformní rovina (superadmin), tenant izolace + CLI |
| `/gk-rate-limiting` | Per-IP rate limit + brute-force lock + extrakce klientské IP |
| `/gk-hardening` | CSRF + security headers + maskování tajemství před error trackerem |

### Frontend (Vue 3 SPA)
| Skill | Co řeší |
|-------|---------|
| `/gk-frontend-fetch` | `apiFetch`/`authFetch`, single-flight refresh + self-heal, access token |
| `/gk-frontend-forms` | Formuláře bez FE validace — chyby z backendu na konkrétní pole |
| `/gk-frontend-ui` | `app-ui` komponenty + composables, router/guards, přísný TS/ESLint/Tailwind |
| `/gk-frontend-grid` | Server-side DataGrid — stránkování/filtry/řazení, výběr + bulk akce; FE stav + BE whitelist/clamp |

### Observabilita
| Skill | Co řeší |
|-------|---------|
| `/gk-logging` | Strukturované `slog` — konstantní klíče, korelace, lint enforcement |
| `/gk-sentry` | Error tracking BE+FE — jen neočekávané chyby, enrichment, source maps |
| `/gk-audit` | Append-only audit security akcí (přežije rollback transakce) |

### Ship & kvalita
| Skill | Co řeší |
|-------|---------|
| `/gk-deploy` | Single binárka, multi-stage Docker, GitHub CI, release + version stamping |
| `/gk-testing` | `testfx` (reálná DB), architektonické konformní testy, quality gate |

## Typické scénáře (kudy začít)

- **„Právě jsem to naklonoval" / „založil jsem repo z template"** → `/gk-init` (u template má krok navíc: `make setup-github`), pak `/gk-architecture`.
- **„Chci přidat endpoint / CRUD"** → `/gk-feature` (provede tě a odkáže na detail).
- **„Padá mi to / nevím proč"** → podle vrstvy: `/gk-errors`, `/gk-permissions`,
  `/gk-repositories`, `/gk-config`.
- **„Background práce"** → viz matice níže.
- **„Vydat / nasadit"** → `/gk-deploy`.

## Background práce — co kdy (run / scheduler / event)

| Potřeba | Mechanismus | Skill | V transakci? |
|---|---|---|---|
| Atomická write op. během requestu | **command** | `/gk-commands` | ✅ ano |
| „Stalo se X" → reakce, co command nezná | **doménový event** | `/gk-domain-events` | ❌ po commitu, sync |
| Krátká fire-and-forget práce, přežije restart (mail/API/webhook, přepočet) | **fire-and-forget run** = `FireAndForget` | `/gk-runs` | ❌ **NE (vynuceno)** |
| Dlouhá práce, co pokračuje po pádu od posledního kroku (import, report) | **durable run** = `Durable` | `/gk-runs` | ❌ **NE (vynuceno)** |
| Periodicky (cron-like) | **scheduler** | `/gk-scheduler` | ❌ inline |

**Klíč:** oba tvary jsou **jeden engine** (`/gk-runs`), oba běží **mimo transakci** (vynuceno) —
**uvnitř transakce nikdy nevolej ven** (SMTP/cizí API drží zámek po dobu volání → zamkne SQLite).
Tvary rozlišuje jediná otázka: **potřebuje to po pádu pokračovat od posledního kroku?**
(ano → run s checkpointem, ne → fire-and-forget run). Detail + diagram →
`docs/framework/background/overview.md` (+ jednotlivé toky v sekcích HTTP / CQRS / Background).

## Práce s dokumentací & psaní skillů
**Při jakékoli práci s dokumentací** (`docs/` — zakládání/úprava stránek, sekce `.list.md`, frontmatter, přesun či přejmenování stránky, nav) sáhni po skillu **`/documan`**. Zná Documan formát i workflow: pravidla frontmatteru (`uri`/`slug`/`parent`, `title` == `# H1`, `layout: 'list'` u sekcí), postup přesunu/přejmenování (uprav uri/slug/parent, přeparentuj potomky, přepiš odkazy) i `make documan-import` / `documan-fix`. Platí to **i pro `gk-*` skilly** — jsou to taky Documan stránky.

Nový `gk-*` skill navíc drž podle vzoru v `_TEMPLATE.md` (vedle tohohle souboru) — struktura (frontmatter + 6 sekcí), jazyk (CZ tělo, EN nadpisy/kód) a pravidla obsahu (pravdivost > marketing, odkazuj neduplikuj).

## Související
- **`/documan`** — pomocník pro práci s dokumentací v `docs/` (Documan formát, sekce, nav, frontmatter, import/fix). **Použij ho vždycky, když saháš na docs.**
- Dokumentace (lidská, web): <https://gokick.strategio.dev/>
- `CLAUDE.md` v rootu — tvrdé invarianty projektu, na které skills odkazují
