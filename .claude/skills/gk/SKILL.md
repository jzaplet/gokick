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

**Jak na to:** napiš `/gk-<název>` (např. `/gk-database` → `/gk-repositories`).
Nebo prostě popiš problém a AI sáhne po správném skillu sama (řídí se podle
`description` každého skillu).

## Cesta projektem (od initu po observabilitu)

### Start & orientace
| Skill | Co řeší |
|-------|---------|
| `/gk-init` | Od `git clone` k běžícímu serveru — install, build, seed admina, dev loop |
| `/gk-config` | `.env` konfigurace: co se čte, kdy, defaulty, kde se validuje |
| `/gk-architecture` | Mentální model: DDD 4 vrstvy, CQRS, dependency rules, bounded contexts |
| `/gk-feature` | Přidat novou featuru **end-to-end** — checklist napříč všemi vrstvami |

### Doména
| Skill | Co řeší |
|-------|---------|
| `/gk-entities` | Entity (db tagy) + value objects s validací + factory funkce |
| `/gk-domain-events` | „Stalo se X" → reakce po commitu, aniž to command handler zná |
| `/gk-errors` | Doménové chyby (Validation/Auth/Permission) → automaticky HTTP status |

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
| `/gk-jobs` | Perzistentní background fronta + worker (přežije restart/crash) |
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

- **„Právě jsem to naklonoval"** → `/gk-init`, pak `/gk-architecture`.
- **„Chci přidat endpoint / CRUD"** → `/gk-feature` (provede tě a odkáže na detail).
- **„Padá mi to / nevím proč"** → podle vrstvy: `/gk-errors`, `/gk-permissions`,
  `/gk-repositories`, `/gk-config`.
- **„Background práce"** → `/gk-jobs` (musí přežít restart) nebo `/gk-scheduler` (periodicky).
- **„Vydat / nasadit"** → `/gk-deploy`.

## Přidání nového gk skillu
Drž se vzoru v `_TEMPLATE.md` (vedle tohohle souboru) — definuje
strukturu (frontmatter + 6 sekcí), jazyk (CZ tělo, EN nadpisy/kód) a pravidla
obsahu (pravdivost > marketing, odkazuj neduplikuj).

## Související
- Dokumentace (lidská, web): <https://gokick.strategio.dev/>
- `CLAUDE.md` v rootu — tvrdé invarianty projektu, na které skills odkazují
