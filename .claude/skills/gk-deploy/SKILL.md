---
name: gk-deploy
description: Vydání aplikace jako jediné samostatné binárky (embedovaný SPA + migrace), multi-stage Dockerfile, GitHub CI (validate/release) a stamping verze do Sentry release. Use when buildíš produkční image, řešíš jak se aplikace vydává/nasazuje, co dělají CLI příkazy serve/worker/seed, nebo odkud se bere verze v Sentry.
layout: 'page'
uri: '/skills/gk-deploy'
position: 10
slug: 'skills-gk-deploy'
parent: 'skills-ship'
navTitle: 'gk-deploy'
title: 'GK — Deploy & release'
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
- Migrace se aplikují **automaticky při startu** přes `database.MigrationManager` (`app/application.go`) — a to **před každým** subcommandem (`serve`, `seed`, `create-user`, `worker`), ne jen při `serve`. Detail: `/gk-migrations`.

### CLI příkazy (`app/presentation/console/`)

Root command `app` (`root.go`) registruje čtyři subcommandy:

| Příkaz | Co dělá |
|---|---|
| `serve` | HTTP server **+** in-process scheduler **+** job worker v jednom procesu (`serve.go`: `scheduler.Run` a `worker.Run` jako goroutiny, sdílí jeden `ctx` ze signal handleru → SIGTERM nechá vše korektně dobíhat) |
| `worker` | Jen perzistentní job worker, bez HTTP a scheduleru (`worker.go`) — pro škálování workeru zvlášť (1 serve replika + N worker replik) |
| `seed` | Vytvoří admin účet (heslo z `APP_SEED_ADMIN_PASSWORD`), pokud ještě není |
| `create-user` | Vytvoří uživatele (`-n` nickname, `-p` heslo, `-e` email, `-r` role); jede přes `SystemCommandBus` (transakce + audit) |

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

- **`validate.yml`** (push na `main` + každý PR): `make install` → `make lint` → `make test` → `make build`. `SKIP_DOCUMAN: "1"` (doc lint má vlastní `documan.yml`).
- **`release.yml`** (na tagu `v*`): postaví produkční image přes multi-stage Dockerfile, stampuje tag jako verzi. **Push do GHCR je defaultně VYPNUTÝ** (gokick je template — fork nesmí auto-publikovat); zapneš repo variable `RELEASE_PUSH=true`. Bez ní se image jen postaví (ověří, že release kompiluje), nepushne. Source-map upload do Sentry je optional (`SENTRY_AUTH_TOKEN` jako build secret).

## Recipe: postavit produkční image lokálně

1. `make docker-build` — postaví `gokick:latest` (Vite SPA → Go binárka → Alpine runtime), verze z `git describe`.
2. Spusť: `docker run -p 3000:3000 -v /host/data:/data -e APP_JWT_SECRET=… gokick:latest`.
3. (První běh) seedni admina: `docker run … gokick:latest seed` (potřebuje `APP_SEED_ADMIN_PASSWORD`).

## Recipe: vydat release přes CI

1. Cut tag z **zelené `main`** (validate prošel): `git tag v1.2.3 && git push origin v1.2.3`.
2. `release.yml` postaví image a stampuje `v1.2.3` jako `main.release` + `VITE_SENTRY_RELEASE`.
3. Pro skutečný push do GHCR měj nastavenou repo variable `RELEASE_PUSH=true` (jinak se image jen postaví).

## Invariants & pitfalls

- **Frontend musíš postavit PŘED Go buildem.** `public/` musí existovat, jinak `//go:embed *` selže. `make build` to řeší pořadím (`build: di fe-build`); Dockerfile tím, že stage `backend` kopíruje `public/` ze stage `frontend`.
- **Žádné `.map` v `public/`.** Source mapy by se vložily do binárky a vystavily = únik zdrojového kódu. Dockerfile guard build shodí, kdyby tam zůstaly.
- **`CGO_ENABLED=0` je záměr** — pure-Go SQLite (`ncruces/go-sqlite3`), statická binárka, žádné CGO, snadný cross-compile.
- **Migrace běží při startu vždy** — nasazení nepotřebuje separátní migrační krok. Detail a `make migrate-*` nástroje: `/gk-migrations`.
- **Sentry je gated na DSN** — bez `APP_SENTRY_DSN` / `APP_SENTRY_DSN_FRONTEND` běží jako no-op. Stamping verze je nezávislý; bez DSN se nikam neposílá.

## Related

- Skills: `/gk-init` (lokální build & rozjetí), `/gk-migrations` (embedované + auto-apply migrace), `/gk-config` (`APP_HTTP_PORT`, `APP_DB_PATH`, `APP_COOKIE_SECURE`, `APP_JWT_SECRET`, `APP_SENTRY_*`), `/gk-jobs` + `/gk-scheduler` (co `serve` / `worker` co-spouští), `/gk-sentry` (co se reportuje a jak verze grupuje chyby)
- Kód: `docker/production/Dockerfile`, `.github/workflows/release.yml`, `.github/workflows/validate.yml`, `Makefile`, `cmd/version.go`, `app/presentation/console/`, `public/embed.go`, `migrations/embed.go`
