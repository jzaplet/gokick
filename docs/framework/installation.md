---
layout: 'page'
uri: '/framework/installation'
position: 10
slug: 'framework-installation'
parent: 'framework'
navTitle: 'Installation'
title: 'Installation'
description: 'Instalace, build, lint, formátování a další make příkazy.'
---

# Installation

Od čistého klonu k běžícímu serveru s admin účtem. Nejrychlejší cesta je skill `/gk-init`, který tě tím provede krok za krokem; tahle stránka je manuální reference těch samých kroků — prerekvizity, instalace a všechny `make` příkazy.

> Po rozběhnutí se podívej na `/gk-init` (a `/gk` pro přehled všech skillů).

## Prerekvizity

| Nástroj | Minimální verze | Ověření |
|---|---|---|
| Go | 1.26+ | `go version` |
| Node.js | 24+ | `node --version` |
| Corepack | (součást Node) | `corepack --version` |
| Make | jakákoliv | `make --version` |

## Instalace

```bash
corepack enable
cp .env.example .env    # upravit APP_JWT_SECRET
make install
make build && make serve
./bin/app seed           # admin účet, heslo z APP_SEED_ADMIN_PASSWORD (povinné)
```

## Make příkazy

### Hlavní

| Příkaz | Co dělá |
|---|---|
| `make build` | Wire DI → Vite build → Go build → `bin/app` |
| `make serve` | Spustí `bin/app serve` |
| `make test` | Vitest (frontend) + go test (app/ + cmd/) |
| `make lint` | ESLint + vue-tsc + knip + golangci-lint + go-arch-lint + golines format-check + ts-check + documan-lint |
| `make format` | ESLint Stylistic fix + golines + documan-fix |

### Vývoj

| Příkaz | Co dělá |
|---|---|
| `make dev` | Rychlý build — Wire DI + Go binárka (bez frontendu) |
| `make fe-dev` | Vite dev server s hot reload + proxy na Go backend |
| `make di` | Regeneruje Wire DI container |

### Migrace

| Příkaz | Co dělá |
|---|---|
| `make migrate-up` | Aplikuje pending migrace |
| `make migrate-down` | Rollback poslední migrace |
| `make migrate-status` | Zobrazí stav migrací |
| `make migrate-create NAME=...` | Vytvoří nový migrační soubor |

### CLI

| Příkaz | Co dělá |
|---|---|
| `./bin/app serve` | HTTP server + in-process scheduler + durable-task worker (sdílí jeden ctx) |
| `./bin/app worker` | Jen persistentní durable-task worker (bez HTTP serveru) |
| `./bin/app seed` | Admin účet (heslo z `APP_SEED_ADMIN_PASSWORD`, povinné) + superadmin (jen když je `APP_SEED_SUPERADMIN_PASSWORD`); s multitenancy on založí adminovi vlastní tenant (`APP_SEED_ADMIN_TENANT`) |
| `./bin/app create-user -n <nick> -p <pass> [-e <email>] [-r admin\|user] [--tenant-id <id> \| --tenant-name <name>]` | Vytvoří uživatele (default role `admin`). S multitenancy on je tenant **povinný** — buď existující `--tenant-id`, nebo nový `--tenant-name` |
| `./bin/app create-superadmin -n <nick> -p <pass> [-e <email>]` | Vytvoří platformního **superadmina** (cross-tenant přístup). Přes API ho založit nelze; druhá (a jediná další) cesta je `seed` s `APP_SEED_SUPERADMIN_PASSWORD` |
| `./bin/app create-tenant -n <name>` | Vytvoří tenant a vypíše jeho id (pro `create-user --tenant-id`) |

> **Multitenancy** je celé popsané ve skillu `/gk-multitenancy` (přepínač, izolace, platformní rovina, tenant tooling).
