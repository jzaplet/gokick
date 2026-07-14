---
layout: 'page'
uri: '/skills/gk-init'
position: 10
slug: 'skills-gk-init'
parent: 'skills-start'
navTitle: 'gk-init'
title: 'GK — Rozjetí projektu (init)'
description: 'Rozjetí projektu po `git clone` — install, build, seed admina a dev loop, aby server běžel a šel se přihlásit. Use when máš čerstvý clone (nebo nový stroj / kolega) a potřebuješ se dostat od nuly k běžícímu serveru s funkčním adminem a frontendem.'
name: 'gk-init'
---

# GK — Rozjetí projektu (init)

Cesta od `git clone` k běžícímu serveru: nainstalovat nástroje, sestavit binárku,
naseedovat admin účet, a vědět, jak točit dev loop.

## What & when

- Sáhni sem, když máš **čerstvý clone** (nebo nový stroj / onboarding kolegy) a chceš
  se dostat k běžícímu serveru, na který se dá přihlásit.
- Pokrývá: `make install`, `make build` vs `make dev`, `make serve`, `make fe-dev`,
  seed admina, a **základ CLI** (`serve` / `seed` / `create-user` / `worker`).
- NEtýká se: konfigurace a env (`/gk-config`), přidávání featur end-to-end
  (`/gk-feature`), ani celkového mentálního modelu vrstev (`/gk-architecture`).
  Hloubku scheduleru / durable enginu tu jen zmíníme — patří do samostatného skillu.

## For non-tech / juniors

Projekt je **jedna binárka** (`bin/app`): Go server, do kterého je „zazipovaný" celý
frontend (Vue 3 SPA). Když ji spustíš, dostaneš web i API z jednoho procesu — žádná
další služba se nemusí pouštět zvlášť.

Aby ta binárka vznikla, je potřeba pár věcí: stáhnout Go a Node knihovny
(`make install`), sestavit frontend i backend (`make build`), a spustit
(`make serve`). Databáze je obyčejný soubor SQLite na disku (`./data/app.db`) — žádný
DB server. **Seed** je jednorázové naplnění DB výchozími daty (admin účet), aby ses měl
čím přihlásit.

## How it works

- **Migrace běží samy.** `Application.Run()` (`app/application.go`) zavolá
  `migrations.RunUp()` **před každým** subcommandem — takže DB schéma se dorovná i při
  `seed` / `create-user`, nejen u `serve`. Ruční `make migrate-up` na první spuštění
  nepotřebuješ.
- **`make build` = plná binárka:** `di` (Wire) → `fe-build` (Vite → `public/`) → Go
  build, který `public/` embeduje. **`make dev` je jen backend** (Makefile řádek ~95:
  `di` + `go build`, **bez** `fe-build`) — rychlý při práci na Go, ale neembeduje SPA.
- **`make serve`** jen spustí `./bin/app serve` — nejdřív musíš mít sestavenou binárku.

  | Příkaz | Co dělá |
  |---|---|
  | `./bin/app serve` | HTTP server **+ in-process scheduler + durable-task worker** v jednom procesu (sdílí jeden `ctx`, SIGTERM nechá vše korektně dobíhat — `console/serve.go`) |
  | `./bin/app seed` | Vytvoří admin účet (pokud chybí), heslo z `APP_SEED_ADMIN_PASSWORD` (`console/seed.go`) |
  | `./bin/app create-user -n <nick> -p <pass> [-e <email>] [-r <role>]` | Vytvoří uživatele; role default `admin`, alternativa `user` (`console/create_user.go`) |
  | `./bin/app create-superadmin -n <nick> -p <pass> [-e <email>]` | Vytvoří platform superadmina (cross-tenant) — jediná cesta k té roli (`console/create_superadmin.go`) |
  | `./bin/app create-tenant -n <name>` | Vytvoří tenant a vypíše jeho id — pro multitenant režim (`console/create_tenant.go`) |
  | `./bin/app worker` | Jen persistentní durable-task worker, bez HTTP serveru — pro škálování workerů zvlášť (`console/worker.go`) |

- **`create-user` jede přes `SystemCommandBus`** (transakce, audit trail, post-commit
  eventy, panic→Sentry — bez Authorize/Tenant middleware) a uvnitř volá stejný
  `*usercmd.CreateUserHandler` jako HTTP API — recykluje stejnou validaci, hashing
  a unique-nickname check.

## Recipe

### Recipe: první rozjetí po clone (plná binárka)

1. `corepack enable` — zapne yarn 4 (Berry), který projekt používá.
2. `cp .env.example .env` — v `.env` **změň `APP_JWT_SECRET`** z placeholderu (≥ 32 znaků)
   a nastav `APP_SEED_ADMIN_PASSWORD` (8–128 znaků), jinak `seed` selže.
3. `make install` — stáhne Go deps, doinstaluje nástroje (`wire`, `golines`,
   `golangci-lint`, `goose`, `go-arch-lint`) a yarn balíčky.
4. `make build` — sestaví frontend i backend → `bin/app` (migrace pak doběhnou samy při
   prvním spuštění).
5. `./bin/app seed` — vytvoří admina.
6. `make serve` — server běží na `http://localhost:3000` (port z `APP_HTTP_PORT`).
   Přihlas se nickem `admin` a heslem z `APP_SEED_ADMIN_PASSWORD`.

### Recipe: dev loop (rychlá iterace)

- **Práce na Go:** `make dev` (backend-only build) → `make serve`. Opakuj po každé změně.
- **Práce na frontendu:** `make fe-dev` — Vite dev server s hot-reloadem na
  `http://localhost:5173`, který proxuje API na běžící `serve` (`:3000`). Drž oba
  procesy zároveň. `APP_CORS_ORIGIN=http://localhost:5173` v `.env` tomu odpovídá.
- Po změně Wire providerů: `make di`. Po změně vrstev: `make arch-check`.

## Invariants & pitfalls

- **`make dev` neembeduje SPA.** Sestaví jen backend (bez `fe-build`) a vygenerované
  bundly v `public/` jsou gitignored — po čistém clone tedy `make dev` + `serve` dá
  server **bez frontendu**. Na funkční web použij `make build` (embed) **nebo**
  `make fe-dev` (Vite na `:5173`).
- **`make serve` nic nebuilduje** — jen spustí `./bin/app`. Když binárka neexistuje
  nebo je stará, nejdřív `make build` / `make dev`.
- **`seed` bez `APP_SEED_ADMIN_PASSWORD` selže.** Proměnná je povinná jen pro `seed`;
  v prostředích, kde seed neběží, ji nech prázdnou.
- **Změň `APP_JWT_SECRET`.** Placeholder v `.env.example` je ≥ 32 znaků — délku vynucuje
  `NewJwtService` (`app/infrastructure/security/jwt.go`): prázdný nebo kratší secret
  shodí start. Ale ponechat výchozí tajemství je bezpečnostní díra — vždy ho přepiš
  na vlastní náhodný řetězec.
- **Nepotřebuješ ruční migraci na první boot** — `RunUp()` ji udělá za tebe před každým
  subcommandem.
- **Go 1.26+ a Node 24+** jsou prerekvizity.

## Related

- Sousední skills: `/gk-config` (env + konfigurace), `/gk-feature` (přidání featury
  end-to-end), `/gk-architecture` (vrstvy a pravidla závislostí), `/gk-bus` (CQRS busy).
- Docs: [Installation](/framework/installation).
- Kód: `app/presentation/console/` (`root.go`, `serve.go`, `seed.go`, `create_user.go`,
  `create_superadmin.go`, `create_tenant.go`, `worker.go`), `app/application.go` (auto-migrace), `Makefile`, `.env.example`.
