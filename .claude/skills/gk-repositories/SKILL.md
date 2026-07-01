---
layout: 'page'
uri: '/skills/gk-repositories'
position: 10
slug: 'skills-gk-repositories'
parent: 'skills-data'
navTitle: 'gk-repositories'
title: 'GK — Repositories (SQLite datová vrstva)'
description: 'Repozitáře nad SQLite — jak píšeš datovou vrstvu, aby sama běžela uvnitř transakce (r.Conn(ctx)), kde jsou vědomé výjimky (raw pool) a proč je DB takhle naladěná. Use when přidáváš/upravuješ repozitář, řešíš proč zápis přežil/nepřežil rollback, nebo ladíš "database is locked" / SQLITE_BUSY.'
name: 'gk-repositories'
---

# GK — Repositories (SQLite datová vrstva)

Jak v gokicku vypadají repozitáře: tenké adaptéry nad SQLite, které se samy zapojí do transakce, plus pár vědomých výjimek a naladění databáze.

## What & when
- Sáhni sem, když píšeš nebo upravuješ repozitář (`app/infrastructure/sqlite/<context>/`) a nevíš, jak má vypadat.
- Když řešíš, **proč zápis přežil nebo nepřežil rollback** (audit, failed-login counter).
- Když ladíš `database is locked` / `SQLITE_BUSY` nebo se ptáš, proč je DSN takový, jaký je.
- NEtýká se to: definice doménového `Repository` interface a entit (to je `/gk-entities`), ani toho, kdo a kdy transakci otevírá (to je `TransactionMiddleware` v `/gk-bus`).

## For non-tech / juniors
Repozitář je **jediné místo, kde se mluví s databází**. Zbytek aplikace řekne „ulož uživatele" a netuší, že pod tím je SQL — to je celé schované tady.

Klíčová vychytávka: jeden požadavek typicky **přečte** data (`FindByNickname`), pak chvíli **počítá** (ověření hesla bcryptem trvá ~200 ms) a teprve pak **zapíše** (`Save`). Aby se to celé chovalo jako jeden nedělitelný blok (transakce — buď projde všechno, nebo nic), musí repozitář umět poznat, že nějaká transakce už běží, a připojit se k ní. To dělá `r.Conn(ctx)` automaticky — ty na to nemyslíš.

Databáze (SQLite — malá databáze v jednom souboru) je naladěná tak, aby se dva souběžní zapisovači (uživatel + úloha na pozadí) nepoprali a aby ten pomalý výpočet uprostřed transakci nerozbil.

## How it works

### r.Conn(ctx) — tx-aware connection
Každý repozitář embeduje `sqlite.BaseRepository` (`app/infrastructure/sqlite/conn.go`) a SQL pouští vždy přes `r.Conn(ctx)`:

```go
type Repository struct { sqlite.BaseRepository }   // embed

func (r *Repository) Save(ctx context.Context, u *user.User) error {
    const q = `INSERT INTO users (...) VALUES (...)`
    _, err := r.Conn(ctx).NamedExecContext(ctx, q, u)   // tx-aware
    return err
}
```

`Conn(ctx)` se podívá do contextu: pokud běží transakce (otevřel ji `TransactionMiddleware`), vrátí `*sqlx.Tx`; jinak vrátí surový pool `*sqlx.DB`. Repo tak nic neví o tom, jestli je v transakci nebo ne — viz `BaseRepository.Conn` v `conn.go` a `TxFromContext` v `app/infrastructure/database/sqlite_manager.go`. SQL se píše přes `sqlx` named queries (`NamedExecContext` mapuje `db:"..."` tagy structu rovnou na `:placeholder`).

### Raw-pool výjimka — zápisy, co musí přežít rollback
Dva zápisy **vědomě obcházejí** `r.Conn(ctx)` a jdou surovým poolem `r.DB.DB()`, aby se uložily (commit) nezávisle na obklopující bus transakci (ta se může vrátit zpět):

- `user.Repository.RecordFailedLogin` a `ResetFailedLogin` (`app/infrastructure/sqlite/user/repository.go`) — počítadlo neúspěšných přihlášení (brute-force ochrana). `LoginHandler` při špatném heslu vrací `AuthError` a vrátí svou transakci zpět (rollback); counter to ale musí přežít, jinak je ochrana k ničemu.
- `audit.Repository.Save` (`app/infrastructure/sqlite/audit/repository.go`) — security audit trail musí zůstat i u commandu, který se vrátí zpět (rollback). Proto Audit middleware sedí **mimo** Transaction middleware (viz `/gk-bus`).

Tohle je **uzavřená množina** tří metod ve dvou repozitářích — žádný jiný repozitář raw pool legitimně nepoužívá a každá výjimka má u metody komentář s důvodem.

### SQLite tuning
`NewSqliteManager` v `app/infrastructure/database/sqlite_manager.go` otevírá pool s DSN:

```
file:<path>?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)
```

- `_txlock=immediate` — každý `BeginTx` startuje jako `BEGIN IMMEDIATE` (vezme write-lock hned na začátku). Bez toho by při default deferred transakci souběžný commit jiného zapisovače během CPU okna (bcrypt) zneplatnil přečtený snapshot a následný zápis by fatálně spadl jako `SQLITE_BUSY_SNAPSHOT` (to `busy_timeout` neumí zopakovat).
- `busy_timeout(5000)` — krátké překryvy mezi zápisy (worker/scheduler vs. uživatel) se serializují čekáním až 5 s místo okamžitého `database is locked`.
- `foreign_keys(on)` — FK je per-connection; přes DSN ho má **každá** nová konexe v poolu (jednorázový `PRAGMA exec` by zapnul jen jednu).
- `file:` prefix je u driveru `ncruces/go-sqlite3` (pure-Go, žádné CGO) povinný — jinak se query string ignoruje.

`journal_mode` se naopak nastavuje **jednorázovým** `PRAGMA journal_mode=...` execem na poolu (ne v DSN), protože WAL se zapisuje do hlavičky souboru a je tedy persistentní. Default je `WAL`, povolené hodnoty `WAL|DELETE|MEMORY` (whitelist přes `APP_DB_JOURNAL_MODE` — chrání proti SQL injection z misconfigurace).

### Aktuální repozitáře
`sqlite/user/` (`user.Repository`), `sqlite/token/` (`token.TokenRepository`), `sqlite/run/` (`run.Repository`), `sqlite/tenant/` (`tenant.Repository`), `sqlite/audit/` (`shared.AuditLogger`), `sqlite/seeder/` (`shared.Seeder`).

## Recipe: nový repozitář
1. Vytvoř `app/infrastructure/sqlite/<context>/repository.go` se `type Repository struct { sqlite.BaseRepository }`.
2. Constructor: `NewRepository(db *database.SqliteManager) *Repository { return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}} }`.
3. Implementuj metody doménového interface (`<context>.Repository` z `app/domain/<context>/`). SQL vždy přes `r.Conn(ctx)`.
4. Konvence not-found: lookupy vracejí `nil, nil` (výjimkou je `user.Repository.FindByID`, který na not-found vrací `*shared.ValidationError`; `run.Repository.FindByID` naopak vrací `nil, nil`).
5. Wire binding v `app/infrastructure/di/container_provider.go`: `wire.Bind(new(<context>.Repository), new(*sqlite<context>.Repository))`, pak `make di`.
6. Pokud je to **nový bounded context**, přidej adresář do `sqlite_repos` v `.go-arch-lint.yml` a spusť `make arch-check` (viz `/gk-feature`).

## Invariants & pitfalls
- **Vždy `r.Conn(ctx)`, nikdy `r.DB.DB()`** — jediná výjimka jsou tři výše uvedené metody (`RecordFailedLogin`, `ResetFailedLogin`, `audit.Save`), každá s komentářem proč. Nový raw-pool zápis bez tohoto důvodu je chyba.
- **Repozitář závisí jen na doménovém interface**, ne na konkrétním infra typu — handlery a CLI nikdy neimportují `*sqlite<context>.Repository`, jen `<context>.Repository`.
- **Nemíchej journal_mode do DSN** — patří do jednorázového `PRAGMA exec`, protože je persistentní v souboru. Pragmy v DSN (`busy_timeout`, `foreign_keys`) jsou per-connection.
- `SQLITE_BUSY_SNAPSHOT` / `database is locked` při ladění obvykle znamená, že někdo obešel `_txlock=immediate` nebo otevírá vlastní konexi mimo manager — nezakládej nový pool, použij `SqliteManager`.

## Related
- `/gk-entities` — definuje doménový `Repository` interface a entity (`db:"..."` tagy), které tahle vrstva implementuje.
- `/gk-bus` — `TransactionMiddleware` je to, co rozhodne, jestli `Conn(ctx)` vrátí transakci; vysvětluje i pořadí Audit vs. Transaction.
- `/gk-feature` — repozitář je krok 2 v checklistu nové featury (domain → repo → command/query → handler → route → DI).
- Kód: `app/infrastructure/database/sqlite_manager.go`, `app/infrastructure/sqlite/conn.go`, `app/infrastructure/sqlite/*/repository.go`
