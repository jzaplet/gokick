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
Migrace žijí v adresáři dialektu — dnes `migrations/sqlite/` — jako `YYYYMMDDHHMMSS_<name>.sql` (Goose SQL formát). Adresář na dialekt je příprava na Postgres adaptér: SQL se mezi enginy liší, ale **verze musí zůstat v lock-stepu**, aby obě DB došly ke stejnému logickému schématu (viz [Plán: PostgreSQL 18 adaptér](/framework/postgres-adapter-plan)).
Aktuální sada = **jediný squashed init** `20260327000001_init_schema.sql` (tabulky `tenants` + unikátní index jména + seed Default tenantu, `users` vč. `lang`, `refresh_tokens`, `audit_log`, `runs` vč. `lang` + všechny indexy) — jako boilerplate gokick dodává aktuální schéma jedním krokem. Historie se squashovala dvakrát: 2026-07-15 (12 kroků vč. vzniku a dropu tabulky `jobs`) a 2026-09-24 (unikátní jméno tenantu, `users.lang`, `runs.lang`). Soubor si drží **první číslo verze**, takže nasazení, která starou historii už aplikovala, mají verzi zapsanou a soubor přeskočí (historické záznamy v `goose_db_version` goose ignoruje). **Pravidlo upgradu:** platí to jen pro nasazení, které prošlo CELOU starou historií — instalace na verzi starší než v1.4.0 musí nejdřív nastartovat některé vydání v1.4.0–v1.4.2 (ta doaplikují zbylé kroky) a teprve pak binárku se squashnutým initem. Projektové migrace přidávej jako NOVÉ soubory za init (`make migrate-create`); vyšší timestamp = běží později.

Existují **dvě oddělené cesty**, jak se migrace spustí:

**1) Embedded auto-up při startu (produkční cesta).**
- `migrations/embed.go` zapéká `*.sql` do binárky přes `//go:embed sqlite/*.sql`
  a vystaví je jako `migrations.SQLite` (`fs.FS` zakořeněný v adresáři dialektu) —
  runtime nepotřebuje žádné soubory na disku.
- `Application.Run` (`app/application.go:24-28`) volá `migrations.RunUp()`
  **před** `rootCmd.Execute(ctx)`. Takže auto-up proběhne při **každém**
  subcommandu — `serve`, `worker`, `seed`, `create-user`, `create-superadmin`,
  `create-tenant` — ne jen u `serve`.
- `sqlite.Migrator.RunUp()` (`app/infrastructure/sqlite/migrator.go`, port `database.Migrator`)
  spustí `Up` goose **Provideru** (`sqlite.NewMigrationProvider`) na embedded FS. **Jen
  směr Up** — automaticky se nikdy nic nerolluje zpět. Provider nemá žádný globální
  goose stav a celý běh pouští na **jednom** `*sql.Conn` — i `-- +goose NO
  TRANSACTION` migrace, takže `PRAGMA foreign_keys=off` při table-rebuildu platí po
  celou dobu přestavby. Connection pool se nezužuje, jeho limit (F-047) zůstává.
- Goose má vlastní logger umlčený (`goose.WithLogger(goose.NopLogger())`); stav se
  hlásí přes aplikační `*slog.Logger`: `migrations: applied {from,to}`,
  `migrations: up to date {version}`, a když po úspěšném Up selže čtení goose
  verze, warn `migrations: applied, but version read failed` — migrace samotné
  proběhly, jen se nefabrikuje rozsah (jedna logovací cesta, viz CLAUDE.md).

**2) Ruční `make migrate-*` (jen vývoj).**
- `make migrate-create / migrate-up / migrate-down / migrate-status` (viz
  `Makefile`) volají **externí `goose` binárku** (`make install` ji nainstaluje)
  nad `migrations/sqlite/` proti DB souboru z `APP_DB_PATH` v `.env`.
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
   případně `make migrate-down` (rollback poslední) při ladění. Pozor: dokud je v adresáři jen init, rollback poslední migrace = **smazání celého schématu**.
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
- Kód: `migrations/sqlite/` (SQL), `migrations/embed.go`,
  `app/infrastructure/sqlite/migrator.go` (`RunUp`),
  `app/application.go` (auto-up při startu), `Makefile` (`migrate-*` targety).
