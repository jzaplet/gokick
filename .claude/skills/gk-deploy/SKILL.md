---
layout: 'page'
uri: '/skills/gk-deploy'
position: 10
slug: 'skills-gk-deploy'
parent: 'skills-ship'
navTitle: 'gk-deploy'
title: 'GK — Deploy & release'
description: 'Vydání aplikace jako jediné samostatné binárky (embedovaný SPA + migrace), multi-stage Dockerfile, GitHub CI (validate/release) a stamping verze do Sentry release. Use when buildíš produkční image, řešíš jak se aplikace vydává/nasazuje, co dělají CLI příkazy serve/worker/seed, nebo odkud se bere verze v Sentry.'
name: 'gk-deploy'
---

# GK — Deploy & release

Celá aplikace je **jeden spustitelný soubor**: Go binárka s embedovaným Vue SPA a SQL migracemi uvnitř. Žádný separátní web server, žádný runtime na asety, žádný krok „pusť migrace ručně". Vydání = postavit tu binárku (lokálně přes `make build`, nebo do Docker image přes multi-stage `Dockerfile`).

## What & when

- Sáhni sem, když: stavíš produkční image (`make docker-build`), chceš pochopit `docker/production/Dockerfile`, ladíš GitHub CI (`validate` / `release` workflow), nebo řešíš, co znamenají CLI příkazy `serve` / `worker` / `seed` / `create-user`.
- Taky když nevíš, **odkud Sentry bere číslo verze** (release stamping) nebo proč je v image jen jeden soubor.
- NEtýká se: lokálního rozjetí po `git clone` (to je `/gk-init`), psaní migrací (`/gk-migrations`), ani jednotlivých env proměnných (`/gk-config`).

## For non-tech / juniors

Představ si, že místo „nahraj na server frontend, backend, databázové skripty a popros admina, ať to spustí ve správném pořadí" dostaneš **jeden soubor**. Ten soubor v sobě nese:

- **frontend** (zkompilovaný Vue web — `public/embed.go`, `//go:embed *`),
- **databázové migrace** (SQL soubory — `migrations/embed.go`, `//go:embed *.sql`),
- a samotný server.

Když ho spustíš (`./app serve`), nejdřív si **sám doženě databázi** (aplikuje chybějící migrace), pak nastartuje web. Nasazení je „zkopíruj jeden soubor a spusť ho". Docker image je jen tenká slupka kolem té binárky, abys ji mohl pustit kdekoli stejně.

Proč to tak je: nic se nemůže rozejít. Frontend vždy odpovídá backendu (jsou v jednom buildu) a databáze je vždy ve verzi, kterou kód čeká (migrace doběhnou při startu).

## How it works

### Single binary (embed)

- `public/embed.go` embeduje výsledek Vite buildu (`//go:embed *` → `embed.FS`), server ho obsluhuje jako SPA.
- `migrations/embed.go` embeduje SQL migrace (`//go:embed *.sql`).
- Migrace se aplikují **automaticky při startu** přes `database.MigrationManager` (`app/application.go`) — a to **před každým** subcommandem (`serve`, `worker`, `seed`, `create-user`, `create-superadmin`, `create-tenant`), ne jen při `serve`. Detail: `/gk-migrations`.

### CLI příkazy (`app/presentation/console/`)

Root command `app` (`root.go`) registruje šest subcommandů:

| Příkaz | Co dělá |
|---|---|
| `serve` | HTTP server **+** in-process scheduler **+** durable-task worker v jednom procesu (`serve.go`: `scheduler.Run` a `worker.Run` jako goroutiny, sdílí jeden `ctx` ze signal handleru → SIGTERM nechá vše korektně dobíhat) |
| `worker` | Jen perzistentní durable-task worker, bez HTTP a scheduleru (`worker.go`; engine v `app/infrastructure/worker/run_worker.go`) — pro škálování workeru zvlášť (1 serve replika + N worker replik) |
| `seed` | Vytvoří admin účet (heslo z `APP_SEED_ADMIN_PASSWORD`), pokud ještě není; s `APP_SEED_SUPERADMIN_PASSWORD` seedne i superadmina, multitenant admin dostane vlastní tenant — vše přes `SystemCommandBus` v jedné transakci |
| `create-user` | Vytvoří uživatele (`-n` nickname, `-p` heslo, `-e` email, `-r` role; multitenant navíc povinně `--tenant-id` pro existující tenant NEBO `--tenant-name`, který založí nový); superadmin roli odmítá (na to je `create-superadmin`); jede přes `SystemCommandBus` (transakce + audit) |
| `create-superadmin` | Vytvoří platformního superadmina (`-n` nickname, `-p` heslo, `-e` email); jede přes `SystemCommandBus`. Roli superadmin **nelze založit přes API** — admin i platform create ji odmítají. Out-of-band cesty jsou dvě: tento příkaz a `seed` s `APP_SEED_SUPERADMIN_PASSWORD` |
| `create-tenant` | Vytvoří tenant a vypíše jeho id (`-n` název) — pro multitenant provisioning. Od té doby, co superadmin plane nabízí `POST /api/v1/platform/tenants`, je to druhá cesta k témuž commandu (`platformcmd.CreateTenantCommand`), ne jediná |

### Production Dockerfile (`docker/production/Dockerfile`)

Tři stage:

1. **`frontend`** (`node:24-alpine`) — `yarn install --immutable` + `yarn build` → Vite SPA do `public/`. Guard na konci selže build, pokud v `public/` zůstaly `.map` soubory (nesmí se embednout a vystavit zdrojový kód).
2. **`backend`** (`golang:1.26-alpine`) — `go build` s `CGO_ENABLED=0` (pure-Go SQLite → plně statická binárka). Kopíruje `public/` ze stage 1, takže SPA i migrace se zaembedují.
3. **`runtime`** (`alpine:3.20`) — jen binárka, **nonroot** uživatel `gokick`, volume `/data` pro SQLite, `ENTRYPOINT ["/gokick/app"]` + `CMD ["serve"]`. `HEALTHCHECK` ťuká `GET http://127.0.0.1:3000/health` (route registrovaná v `server.go`, public).

`wire_gen.go` je commitnutý — uvnitř image se Wire nepouští. Lokálně: `make docker-build` (volá Dockerfile s `--build-arg VERSION=$(VERSION)`).

### Version stamping (Sentry release)

Jedno číslo verze teče do binárky i do SPA, aby Sentry grupoval chyby podle nasazené verze:

- `Makefile`: `VERSION ?= $(shell git describe --tags --always --dirty)`.
- `make build` ho linkne do binárky přes `-ldflags "-X main.release=$(VERSION)"` a do SPA přes `VITE_SENTRY_RELEASE`.
- `cmd/version.go`: `releaseVersion(envRelease)` — linker-injected `main.release` má přednost; fallback na `APP_SENTRY_RELEASE` z env (deploy může verzi dodat i bez stampingu). Když není ani jedno, Sentry nechá release prázdný (to mu nevadí).
- Release workflow tuhle hodnotu **přepíše git tagem** (`VERSION=${{ github.ref_name }}`).

(Pozn.: cobra `--version` v `root.go` je statický string `"0.1.0"` — nesouvisí se Sentry release; ten je `main.release`.)

### GitHub CI

- **`validate.yml`** (**jen na PR** + `workflow_call` z `release.yml`): job `validate` = `make install` → `make lint` → `make test` → `make build`; paralelní job `e2e` = `make e2e` (durable-run process-lifecycle testy, viz `/gk-runs`). `SKIP_DOCUMAN: "1"` (doc lint má vlastní `documan.yml`). Na push do `main` se **nespouští**: PR se rebase-merguje, takže na main přistane strom, který tenhle job na PR už proběhl. Nový push do PR ten předchozí běh zruší (concurrency per ref).
- **`commitlint.yml`** (na PR): prožene každý commit v PR přes `commitlint` — server-side půlka vynucení Conventional Commits (lokální `commit-msg` hook jde obejít `--no-verify`, tohle ne). Konvence commitů/větví: `CONTRIBUTING.md`.
- **`release-please.yml`** (push na `main`): jede na `RELEASE_PLEASE_TOKEN`, když je secret nastavený (jinak `GITHUB_TOKEN`) — a právě na tom závisí, jestli release PR dostane CI checky, tedy jestli jdou zapnout required status checks. Drží otevřený „release PR", počítá další SemVer z commitů a přepisuje `CHANGELOG.md`; jeho merge vytvoří tag `vX.Y.Z` a GitHub Release. Verzování je automatické — verze ani changelog se needitují ručně.
- **`release.yml`** (`workflow_call` z release-please **nebo** ruční `workflow_dispatch` s tagem — `on: push: tags` schválně NENÍ, viz hlavička souboru): nejdřív job `validate` — přes `workflow_call` prožene celý `validate.yml` (lint + test + build + E2E) nad **přesně tím commitem, který se vydává**; teprve když projde, job `image` postaví produkční image přes multi-stage Dockerfile a stampuje tag jako verzi. Ta brána je jediné, co release commit spolehlivě validuje: `validate.yml` jinak jede jen na PR, a co dostane release PR od release-please, je nepředvídatelné — GitHub rozjíždí „approve bot PRs" a gokick naměřil obojí (PR #39/#41 běhy dostaly ve stavu `action_required`, #43/#55 nedostaly nic). Gate proto na tom nestojí. Pozor na důsledek: release-please **tagne dřív**, než tenhle workflow zavolá, takže selhání v bráně nechá tag a GitHub Release bez image — viditelně nedodělaný release je menší zlo než tiše nevalidovaný; **nápravu čti v hlavičce `release.yml`, tlačítko „Re-run" ji neudělá**. `on: push: tags` schválně chybí: s `GITHUB_TOKEN` stejně nikdy nespustilo nic, ale jakmile release-please dostane PAT, začalo by střílet **vedle** volání z release-please a jeden tag by se postavil a pushnul dvakrát. **Push do GHCR je defaultně VYPNUTÝ** (gokick je template — fork nesmí auto-publikovat); zapneš repo variable `RELEASE_PUSH=true`. Bez ní se image jen postaví (ověří, že release kompiluje), nepushne. Source-map upload do Sentry je optional (`SENTRY_AUTH_TOKEN` build secret + `SENTRY_ORG`/`SENTRY_PROJECT` vars) — celý recept je v `/gk-sentry`.

## Recipe: postavit produkční image lokálně

1. `make docker-build` — postaví `gokick:latest` (Vite SPA → Go binárka → Alpine runtime), verze z `git describe`.
2. Spusť: `docker run -p 3000:3000 -v /host/data:/data -e APP_JWT_SECRET=… gokick:latest`.
3. (První běh) seedni admina: `docker run … gokick:latest seed` (potřebuje `APP_SEED_ADMIN_PASSWORD`).

## Recipe: vydat release (release-please, doporučeno)

1. **Rebase-merguj** `feat:` / `fix:` PR do `main` (merge i squash jsou na repu vypnuté — každý commit přistane jednotlivě, tak je drž čisté). Nic dalšího neděláš.
2. release-please drží otevřený PR „chore(main): release X.Y.Z" a průběžně v něm dopočítává verzi + `CHANGELOG.md` z commitů. Nech ho otevřený, dokud nechceš vydat.
3. **Rebase-merge toho release PR = vydání.** release-please tagne `vX.Y.Z` + založí GitHub Release; tag pak přes `workflow_call` postaví image (`release.yml`). Verze teče do binárky (`main.release`) i SPA (`VITE_SENTRY_RELEASE`) → Sentry release, stejně jako dřív.
4. Pro skutečný push do GHCR měj nastavenou repo variable `RELEASE_PUSH=true` (jinak se image jen postaví).

Konvence commitů (typ = bump) a větví: `CONTRIBUTING.md`.

## Recipe: ruční release (escape hatch)

Když potřebuješ vydat mimo release-please flow: tag musí existovat (`git tag v1.2.3 && git push origin v1.2.3`) a build spustíš **ručně** — `gh workflow run release.yml -f tag=v1.2.3`. Samotný push tagu **nic nespustí** (`on: push: tags` v `release.yml` schválně není). Jede to stejnou cestou včetně `validate` brány, takže tag z rozbité `main` image nepostaví (spadne na bráně a nechá tag bez artefaktu). Používej jen výjimečně; běžná cesta je release PR výše.

## Recipe: pull image na deploy target (private vs public GHCR)

Build+push a **pull** jsou dva oddělené světy — credentials řešíš jen v tom druhém:

- **Push (CI)** — `release.yml` se loguje do GHCR vestavěným `GITHUB_TOKEN` (`packages: write`). Ten umí vždy pushnout do package **vlastního** repa — v private repu identicky jako v public. **Na push žádný secret nenastavuješ**; jen zapni `RELEASE_PUSH=true` (default vypnuto, viz výše).
- **Pull (deploy target, např. Dokploy)** — tady je rozdíl. **První push vytvoří GHCR package jako PRIVATE** (bez ohledu na to, že jsi push zapnul — viz hlavička `release.yml`). U public gokicku je package ručně přepnutý na public, proto ho Dokploy táhne anonymně bez creds. U **private** repa package zůstane neanonymní → deploy target se musí čím autentizovat. Dvě cesty:
  - **A) Package public** — Package → Package settings → Change visibility → Public. Repo zůstane private, veřejně jde stáhnout jen zkompilovaný image. Pull pak bez creds. Kompromis: artefakt (ne zdroják) je světu k dispozici.
  - **B) Package private + token** (doporučeno pro produkt) — GitHub PAT se scope `read:packages`. V Dokploy u aplikace → Docker Provider vyplň `registryUrl=ghcr.io`, `username=<gh-user>`, `password=<PAT>` (ta tři pole, co u public pullu zůstávají prázdná). Nic veřejného, jednorázové nastavení.

`make setup-github` na to upozorní: u ne-public repa na konci vypíše, že package není anonymně stažitelný, a odkáže sem.

## Invariants & pitfalls

- **Frontend musíš postavit PŘED Go buildem.** `public/` musí existovat, jinak `//go:embed *` selže. `make build` to řeší pořadím (`build: di fe-build`); Dockerfile tím, že stage `backend` kopíruje `public/` ze stage `frontend`.
- **Žádné `.map` v `public/`.** Source mapy by se vložily do binárky a vystavily = únik zdrojového kódu. Dockerfile guard build shodí, kdyby tam zůstaly.
- **`CGO_ENABLED=0` je záměr** — pure-Go SQLite (`ncruces/go-sqlite3`), statická binárka, žádné CGO, snadný cross-compile.
- **Migrace běží při startu vždy** — nasazení nepotřebuje separátní migrační krok. Detail a `make migrate-*` nástroje: `/gk-migrations`.
- **Sentry je gated na DSN** — bez `APP_SENTRY_DSN` / `APP_SENTRY_DSN_FRONTEND` běží jako no-op. Stamping verze je nezávislý; bez DSN se nikam neposílá.

## Related

- Skills: `/gk-init` (lokální build & rozjetí), `/gk-migrations` (embedované + auto-apply migrace), `/gk-config` (`APP_HTTP_PORT`, `APP_DB_PATH`, `APP_COOKIE_SECURE`, `APP_JWT_SECRET`, `APP_SENTRY_*`), `/gk-runs` + `/gk-scheduler` (co `serve` / `worker` co-spouští), `/gk-sentry` (co se reportuje a jak verze grupuje chyby)
- Workflow & release: `CONTRIBUTING.md` (větve, commity, release flow), `commitlint.config.js`, `lefthook.yml`, `release-please-config.json`, `.release-please-manifest.json`, `scripts/setup-github.sh` (bootstrap nového repa z template — viz `/gk-init`)
- Kód: `docker/production/Dockerfile`, `.github/workflows/release.yml`, `.github/workflows/release-please.yml`, `.github/workflows/commitlint.yml`, `.github/workflows/validate.yml`, `Makefile`, `cmd/version.go`, `app/presentation/console/`, `public/embed.go`, `migrations/embed.go`
