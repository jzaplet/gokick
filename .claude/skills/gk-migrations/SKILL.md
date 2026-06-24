---
layout: 'page'
uri: '/skills/gk-migrations'
position: 20
slug: 'skills-gk-migrations'
parent: 'skills-data'
navTitle: 'gk-migrations'
title: 'GK — Database migrations (Goose)'
description: 'Databázové migrace přes Goose — SQL soubory embedované do binárky, automaticky aplikované při startu, plus make migrate-* nástroje pro vývoj. Use when přidáváš/měníš tabulku nebo sloupec, zakládáš novou entitu v DB, nebo řešíš „proč jsou migrace dvakrát — make i automaticky".'
name: 'gk-migrations'
---

# GK — Database migrations (Goose)

Jak se v gokicku mění schéma databáze: verzované SQL soubory (Goose formát),
zapečené do binárky a spuštěné automaticky při každém startu — plus ruční
`make migrate-*` nástroje pro lokální vývoj.

## What & when
- Sáhni sem, když přidáváš nebo měníš tabulku/sloupec/index, zakládáš novou
  perzistovanou entitu (nová tabulka), nebo nevíš, jak migraci napsat a spustit.
- Sáhni sem i pro pochopení, **proč existují dvě cesty** — automatická při startu
  a ruční přes `make` — a kdy kterou použít.
- NEtýká se: doménového modelu entity / value objektů (`/gk-entities`),
  čtení/zápisu do tabulky a ladění transakcí (`/gk-repositories`), end-to-end
  přidání featury přes všechny vrstvy (`/gk-feature`).

## For non-tech / juniors
**Migrace** je jeden malý SQL soubor, který popisuje **jednu změnu schématu**
databáze — „přidej tabulku `jobs`", „přidej sloupec `locked_until` do `users`".
Každá migrace má **timestamp v názvu**, takže se spouští ve správném pořadí,
a má dvě části: `Up` (jak změnu provést) a `Down` (jak ji vrátit zpět).

Nástroj **Goose** si v databázi pamatuje, které migrace už proběhly (v tabulce
`goose_db_version`). Při startu aplikace pustí jen ty nové. Tím má každý vývojář
i produkce přesně stejné schéma — nikdo nemusí ručně rozjíždět SQL příkazy.

Analogie: migrace jsou jako verze nábytkového návodu — krok po kroku, očíslované.
Goose si značí, u kterého kroku jsi skončil, a dorazí jen ty zbývající.

## How it works
Migrace žijí v `migrations/` jako `YYYYMMDDHHMMSS_<name>.sql` (Goose SQL formát).
Aktuální sada: `20260327000001_init_schema.sql` (tabulky `users`, `refresh_tokens`),
`20260517000001_create_jobs_table.sql`, `20260517000002_add_user_lock_columns.sql`,
`20260517000003_create_audit_log.sql`. Vyšší timestamp = běží později.

Existují **dvě oddělené cesty**, jak se migrace spustí:

**1) Embedded auto-up při startu (produkční cesta).**
- `migrations/embed.go` zapéká všechny `*.sql` do binárky přes `//go:embed *.sql`
  (`var FS embed.FS`) — runtime nepotřebuje žádné soubory na disku.
- `Application.Run` (`app/application.go:24-28`) volá `migrations.RunUp()`
  **před** `rootCmd.Execute(ctx)`. Takže auto-up proběhne při **každém**
  subcommandu — `serve`, `worker`, `seed`, `create-user` — ne jen u `serve`.
- `MigrationManager.RunUp()` (`app/infrastructure/database/migration_manager.go`)
  spustí `goose.UpContext` na embedded FS. **Jen směr Up** — automaticky se nikdy
  nic nerolluje zpět.
- Goose má vlastní logger umlčený (`goose.SetLogger(goose.NopLogger())`); stav se
  hlásí přes aplikační `*slog.Logger`: `migrations: applied {from,to}` nebo
  `migrations: up to date {version}` (jedna logovací cesta, viz CLAUDE.md).

**2) Ruční `make migrate-*` (jen vývoj).**
- `make migrate-create / migrate-up / migrate-down / migrate-status` (viz
  `Makefile`) volají **externí `goose` binárku** (`make install` ji nainstaluje)
  proti DB souboru z `APP_DB_PATH` v `.env`.
- Tady žijí `down` a `status` — aplikace je sama nikdy nepouští.
- Obě cesty sdílí stejnou DB i Goose tabulku `goose_db_version`, takže verze
  zůstávají konzistentní (auto-up dožene to, co `make` nepustil).

## Recipe

### Recipe: přidat migraci
1. `make migrate-create NAME=add_orders_table` → vznikne
   `migrations/<timestamp>_add_orders_table.sql` s prázdnými `Up`/`Down` bloky.
2. Vyplň **oba** bloky. Změna i její opak:
   ```sql
   -- +goose Up
   ALTER TABLE users ADD COLUMN locked_until DATETIME;

   -- +goose Down
   ALTER TABLE users DROP COLUMN locked_until;
   ```
3. Lokálně ověř: `make migrate-up` (aplikuj) → `make migrate-status` (zkontroluj),
   případně `make migrate-down` (rollback poslední) při ladění.
4. Protože je SQL embedovaná do binárky **při kompilaci**, znovu binárku přelož
   (`make dev`) a teprve pak spusť (`make serve`) — auto-up novou migraci dožene
   sám. Bez přeložení běží stará binárka, která novou migraci ještě nemá zapečenou.
   Soubor commitni.

## Invariants & pitfalls
- **Automaticky běží jen `Up`.** `down` je lokální vývojový rollback, nikdy ne
  produkční cesta — produkce schéma jen dopředně dotahuje.
- **Vždy napiš i `Down` blok.** Goose `down` ho potřebuje; bez něj rollback selže.
- **Vyšší timestamp běží později.** Nikdy needituj už nasazenou migraci — udělej
  novou. Editace minulé migrace neproběhne, protože ji Goose má za hotovou.
- **`make migrate-*` čte `.env`** (`APP_DB_PATH`) a sahá na vývojovou DB — je to
  oddělené od embedded startup cesty. Bez `.env` tyto targety nefungují.
- **Nová perzistovaná entita = nová migrace.** Tenhle krok feature-checklist
  v CLAUDE.md explicitně nejmenuje — snadno se zapomene. Tabulkové sloupce musí
  ladit s `db:"..."` tagy entity (`/gk-entities`).
- **Konvence:** `init_schema` používá `CREATE TABLE IF NOT EXISTS` + idempotentní
  `DROP ... IF EXISTS` v `Down`. Drž stejný styl u nových migrací.

## Related
- Sousední skills: `/gk-entities` (entita ↔ sloupce tabulky, `db:` tagy),
  `/gk-repositories` (čtení/zápis do migrované tabulky, transakce),
  `/gk-feature` (přidání featury end-to-end — migrace je její součást).
- Kód: `migrations/` (SQL + `embed.go`),
  `app/infrastructure/database/migration_manager.go` (`RunUp`),
  `app/application.go` (auto-up při startu), `Makefile` (`migrate-*` targety).
