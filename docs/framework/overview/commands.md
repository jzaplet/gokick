---
layout: 'page'
uri: '/framework/overview/commands'
position: 2
slug: 'framework-overview-commands'
parent: 'framework-overview'
navTitle: 'Installation'
title: 'Installation'
description: 'Instalace, build, lint, formátování a další make příkazy.'
---

# Installation

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
| `make lint` | ESLint + vue-tsc + golangci-lint + go-arch-lint + documan-lint |
| `make format` | ESLint Stylistic fix + golines + documan-fix |

### Vývoj

| Příkaz | Co dělá |
|---|---|
| `make dev` | Quick build -- Wire DI + Go binary (bez frontendu) |
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
| `./bin/app serve` | Spustí HTTP server |
| `./bin/app seed` | Naplní DB výchozími daty (admin user) |
| `./bin/app create-user -n <nick> -p <pass> [-e <email>] [-r <role>]` | Vytvoří uživatele (výchozí role `admin`) |
