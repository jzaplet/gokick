---
layout: 'page'
uri: '/framework/installation'
position: 20
slug: 'framework-installation'
parent: 'framework'
navTitle: 'Installation'
title: 'Installation'
description: 'Instalace, build, lint, formátování a další make příkazy.'
---

# Installation

Od čistého klonu k běžícímu serveru s admin účtem. Nejrychlejší cesta je skill `/gk-init`, který tě tím provede krok za krokem; tahle stránka je manuální reference těch samých kroků — prerekvizity, instalace a všechny `make` příkazy.

> Po rozběhnutí se podívej na `/gk-init` (a `/gk` pro přehled všech skillů).

> **Založil sis repo z template („Use this template")?** Lokální instalace níže tě dostane k běžícímu serveru, ale **releasy a branch protection zůstanou nenakonfigurované**, dokud neproběhne [bootstrap z template](#nový-projekt-z-template-bootstrap) — je to krok navíc, který se z template nepřenese.

## Prerekvizity

| Nástroj | Minimální verze | Ověření |
|---|---|---|
| Go | 1.26+ | `go version` |
| Node.js | 24+ | `node --version` |
| Corepack | (součást Node) | `corepack --version` |
| Make | jakákoliv | `make --version` |
| GitHub CLI `gh` | jen pro bootstrap z template | `gh auth status` (přihlášený, admin na repu) |

## Instalace

```bash
corepack enable
cp .env.example .env    # upravit APP_JWT_SECRET (≥32 znaků) A nastavit APP_SEED_ADMIN_PASSWORD (jinak `seed` selže)
make install            # + nadrátuje git hooky (commit-msg / pre-push)
make build && make serve
./bin/app seed           # admin účet, heslo z APP_SEED_ADMIN_PASSWORD (povinné)
```

## Nový projekt z template (bootstrap)

Když je repo založené z gokick template, přenesou se **soubory** (workflowy, hooky config, release-please config) a CI běží hned — ale **nastavení repa** a baseline verze se nepřenášejí. Po `make install` výše (které nadrátuje lokální git hooky) proto ještě jednou spusť:

```bash
make setup-github ARGS="--reset-version 0.1.0"
```

Zapne **Actions write + create-PR permissions** (bez nich release-please neotevře release PR ani nepushne tag), založí **branch ruleset** na `main` (vyžadovat PR, zákaz force-push/mazání) a `--reset-version` přepíše `.release-please-manifest.json` z gokickovy verze na tvoji výchozí — ten commitni přes PR. Bez `--reset-version` příkaz proběhne, ale verze zůstane na gokickově (`1.1.0`). Detail: `scripts/setup-github.sh --help`, kompletní recept `/gk-init`.

> Ruleset má dva profily. **Default je „lehčí"**: vyžaduje PR, blokuje force-push/mazání a povoluje jen rebase merge, ale **nevynucuje zelené CI**. Důvod: co dostane release PR od release-please (otevřený přes `GITHUB_TOKEN`) za checky, je dnes nepředvídatelné — GitHub rozjíždí „bot-created PRs can run workflows if approved" a gokick naměřil obojí, běhy ve stavu `action_required` i vůbec nic. Required check by proto release PR buď zablokoval, nebo si vyžádal klik na každé vydání.
>
> **Na ostrém projektu použij `make setup-github ARGS="--release-token"`** — uloží token jako secret `RELEASE_PLEASE_TOKEN` (release PR pak checky dostává spolehlivě) a teprve pak přidá **required status checks + strict** policy, tedy „bez rebase a zeleného CI to nemergneš". Token chce repo práva Contents / Pull requests / Issues: read+write.
>
> ⚠️ **Nepleť si ho s pull tokenem pro GHCR.** Ten je jiný, žije na deploy targetu (ne v repu), stačí mu `read:packages` a bez něj nestáhneš privátní image. A na to, aby se image vůbec pushnul, potřebuješ repo variable `RELEASE_PUSH=true`.

## Make příkazy

### Hlavní

| Příkaz | Co dělá |
|---|---|
| `make build` | Wire DI → Vite build → Go build → `bin/app` |
| `make serve` | Spustí `bin/app serve` |
| `make test` | Vitest (frontend) + go test (app/ + cmd/) |
| `make lint` | ESLint + vue-tsc + knip + golangci-lint + go-arch-lint + golines format-check + ts-check + boundary-check + errfields-check + i18n-check + docpaths-check + documan-lint |
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
