---
layout: 'page'
uri: '/skills/gk-init'
position: 10
slug: 'skills-gk-init'
parent: 'skills-start'
navTitle: 'gk-init'
title: 'GK — Rozjetí projektu (init)'
description: 'Rozjetí projektu po `git clone` NEBO z GitHub template („Use this template") — install, build, seed admina, dev loop a jednorázový GitHub bootstrap `make setup-github` (Actions permissions + branch ruleset + reset verze), který se z template nepřenese. Use when máš čerstvý clone nebo nový repo z template/boilerplate a potřebuješ se dostat od nuly k běžícímu serveru A správně nakonfigurovanému repu (git hooky, releasy, branch protection, verzování).'
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
  | `./bin/app create-superadmin -n <nick> -p <pass> [-e <email>]` | Vytvoří platform superadmina (cross-tenant) — roli nelze založit přes API, jen out-of-band: tímto příkazem (`console/create_superadmin.go`) nebo `seed` s `APP_SEED_SUPERADMIN_PASSWORD` |
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

### Recipe: nový projekt z template (gokick jako boilerplate)

gokick je GitHub **template**. „Use this template" → nový repo dostane všechny
**soubory** (workflowy, `lefthook.yml`, `commitlint.config.js`, release-please
config, …) a CI se rozběhne samo (Actions jsou u template-repa zapnuté, na rozdíl
od forků). Ale **nastavení repa** a **lokální git hooky** se nepřenášejí — musíš je
jednou dorovnat:

1. Naklonuj nový repo a projeď „první rozjetí po clone" výše (`make install` mimo
   jiné nadrátuje git hooky — `.git/hooks` není v repu, takže bez tohoto kroku
   commit-msg + pre-push hook neběží; do té doby commity chytá CI `commitlint.yml`).
2. `make setup-github ARGS="--reset-version 0.1.0"` — jednorázový bootstrap
   (potřebuje `gh` login s adminem na repu). Zapne **Actions write + create-PR
   permissions** (bez nich release-please neotevře release PR ani nepushne tag),
   nastaví **rebase-only** merge a založí **branch ruleset** na `main` (vyžadovat PR,
   zákaz force-push/mazání, jen rebase merge). `--reset-version` přepíše
   `.release-please-manifest.json` z gokickovy verze na tvoji výchozí (jinak by tvůj
   první release navázal na gokickovu verzi) — commitni ho pak přes PR.
   Detail: `scripts/setup-github.sh --help`.
3. **Chceš, aby release opravdu vydal image? Tohle si projdi, ať se pak nedivíš.**
   Jsou to tři NEZÁVISLÉ věci a pletou se dohromady:

   | Co | Bez toho | Kde se nastavuje |
   |---|---|---|
   | `RELEASE_PUSH=true` | image se **jen postaví a zahodí** — nikam se nepushne | `gh variable set RELEASE_PUSH --body true` |
   | **pull token** na deploy targetu | privátní GHCR package **nejde stáhnout** → deploy neproběhne | u deploy targetu (registry credentials), NE v repu; classic PAT se scope `read:packages` |
   | **release-please token** (volitelný) | release PR nedostane spolehlivě checky → nejdou zapnout required checks | `make setup-github ARGS="--release-token"` |

   U **privátního** repa jsou první dvě položky povinné, jinak nemáš co deploynout.
   Třetí je komfort, ne podmínka buildu — viz bod 4.
4. **Přísnější režim — VOLITELNÝ, rozmysli si ho.** Bez něj projekt normálně funguje;
   default z kroku 2 stačí pro sólo práci i pro experimenty.

   ```bash
   make setup-github ARGS="--release-token"
   ```

   Vyžádá si token (skrytý vstup / `$RELEASE_PLEASE_TOKEN`), uloží ho jako repo secret
   `RELEASE_PLEASE_TOKEN` a **teprve pak** přidá do rulesetu **required status checks +
   strict** policy.

   **Co tím získáš:** otevřený PR nepůjde mergnout, dokud není rebasnutý na aktuální
   `main` a znovu zelený. To je jediná věc, která ochrání před „dva PR si navzájem
   rozbily main, každý zvlášť byl zelený".

   **Kdy to zapnout:** pracuje vás na projektu víc, nebo běžně máte otevřených několik
   PR naráz.

   **Kdy to NEzapínat:** děláš sólo a mergueš po jednom (přínos je pak skoro nulový),
   nebo **nemáš ten token** — a to je důležité: bez tokenu otevírá release PR
   `GITHUB_TOKEN` a co pak ten PR dostane za checky, je dnes **nepředvídatelné**.
   GitHub rozjíždí „bot-created PRs can run workflows if approved" a gokick naměřil
   obojí: běhy ve stavu `action_required` (musíš je odkliknout) i vůbec žádné. Required
   check přijímá jen `success`/`skipped`/`neutral`, takže bez tokenu si buď zasekneš
   release PR napořád, nebo si přidáš klik ke každému vydání — a dopředu nepoznáš který.
   **Proto skript required checks bez tokenu nezapne**, i kdybys chtěl.

   **Co NEztrácíš, když to nezapneš:** release se pořád nedá vydat nevalidovaný.
   `release.yml` si sám prožene celou sadu nad tím commitem, který se vydává, a teprve
   pak staví image — a to platí v obou režimech.

   Token potřebuje repo práva **Contents / Pull requests / Issues: read+write**
   (classic PAT: scope `repo`). **Není to ten samý token jako pull token pro GHCR** —
   ten patří na deploy target a stačí mu `read:packages`.
5. (Volitelně) Sentry: `SENTRY_*` vars + `SENTRY_AUTH_TOKEN`.

Od té chvíle běží plný workflow: Conventional Commits (lokálně hook + CI), branch
naming, a release-please řídí verze + `CHANGELOG.md`. Konvence: `CONTRIBUTING.md`;
release mechanika: `/gk-deploy`.

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
- **Z template se nepřenášejí nastavení ani hooky, jen soubory.** Branch ruleset a
  Actions permissions jsou nastavení repa (řeší `make setup-github`); git hooky jsou
  lokální (řeší `make install`); `.release-please-manifest.json` nese gokickovu verzi
  (řeší `--reset-version`). Repo variables (`RELEASE_PUSH`, `SENTRY_*`) taky ne — ale
  `release.yml` je defaultně safe-off, takže bez nich se image jen postaví, nepushne.

## Related

- Sousední skills: `/gk-config` (env + konfigurace), `/gk-feature` (přidání featury
  end-to-end), `/gk-architecture` (vrstvy a pravidla závislostí), `/gk-bus` (CQRS busy).
- Docs: [Installation](/framework/installation).
- Workflow: `CONTRIBUTING.md` (větve, commity, release), `/gk-deploy` (release mechanika).
- Kód: `app/presentation/console/` (`root.go`, `serve.go`, `seed.go`, `create_user.go`,
  `create_superadmin.go`, `create_tenant.go`, `worker.go`), `app/application.go` (auto-migrace), `Makefile`, `scripts/setup-github.sh`, `.env.example`.
