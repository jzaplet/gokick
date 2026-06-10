---
layout: 'page'
uri: '/framework/infrastructure/database'
position: 2
slug: 'framework-infrastructure-database'
parent: 'framework-infrastructure'
navTitle: 'Database'
title: 'Database'
description: 'Balíčky database/ a sqlite/ -- SqliteManager, migrace, BaseRepository, repozitáře.'
---

# Database

## Proč

Databázová vrstva je rozdělena na dvě části: `database/` (správa připojení, transakce, migrace) a `sqlite/` (repozitáře implementující doménové interfaces). Pure-Go SQLite driver `ncruces/go-sqlite3` -- žádné CGO.

## Jak

### SqliteManager

```go
// infrastructure/database/sqlite_manager.go

type SqliteManager struct { /* sqlx.DB wrapper */ }

func NewSqliteManager(cfg *config.Config) (*SqliteManager, error)
func (m *SqliteManager) DB() *sqlx.DB
func (m *SqliteManager) Close() error
```

Při vytvoření otevírá pool s DSN `file:<path>?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)` a separátně zapne `PRAGMA journal_mode=WAL|DELETE|MEMORY` (whitelistované přes `APP_DB_JOURNAL_MODE`). Implementuje `shared.Transactor` interface (duck typing) -- používá ho `TransactionMiddleware`.

- `_txlock=immediate` -- každý `BeginTx` startuje jako `BEGIN IMMEDIATE`, ne deferred. Bus handler typicky čte (`FindBy…`) → CPU drží (např. bcrypt 200 ms) → zapisuje (`Save`). Pod default deferred-tx by mezitím commitnutý sourozenec writer (worker poll, scheduler) zneplatnil read snapshot a follow-up zápis by selhal okamžitě jako `SQLITE_BUSY_SNAPSHOT` (busy_timeout to nepokrývá, je to fatal-to-tx). IMMEDIATE bere write lock už při BEGIN -- snapshot zůstává validní celou dobu handleru.
- `busy_timeout(5000)` -- překryvy mezi worker/scheduler writy a user requestem serializují čekáním až 5 s místo "database is locked".
- `foreign_keys(on)` v DSN, ne jako `PRAGMA exec` -- FK je per-connection v SQLite a `db.Exec("PRAGMA foreign_keys=ON")` by ho zapnul jen na jedné poolované konexi. DSN zaručuje, že každá nově otevřená conn FK má.
- `file:` prefix je u ncruces driveru povinný -- bez něj query string ignoruje.

Transakce v contextu:

```go
// database/sqlite_manager.go
func (m *SqliteManager) BeginTx(ctx context.Context) (context.Context, error)
func (m *SqliteManager) Commit(ctx context.Context) error
func (m *SqliteManager) Rollback(ctx context.Context) error
func TxFromContext(ctx context.Context) *sqlx.Tx
```

### Migrace (Goose)

`MigrationManager` embeduje SQL migrace z `/migrations/` do binárky a spouští je automaticky při každém spuštění CLI (`Application.Run`). Boilerplate startuje s jednou konsolidovanou migrací `20260327000001_init_schema.sql`, která zakládá tabulky `users` a `refresh_tokens`. Další migrace se přidávají s vyšším timestampem.

```sql
-- migrations/20260327000001_init_schema.sql

-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    nickname TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_refresh_tokens_user_id;
DROP INDEX IF EXISTS idx_refresh_tokens_token_hash;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
```

```bash
make migrate-create NAME=xxx   # Nová migrace
make migrate-up                # Aplikuj
make migrate-down              # Rollback
make migrate-status            # Stav
```

### BaseRepository a Conn interface

Všechny repozitáře embedují `sqlite.BaseRepository`, který resolvuje transakci z contextu:

```go
// infrastructure/sqlite/conn.go

type Conn interface {
    NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error)
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    GetContext(ctx context.Context, dest any, query string, args ...any) error
    SelectContext(ctx context.Context, dest any, query string, args ...any) error
}

type BaseRepository struct {
    DB *database.SqliteManager
}

func (b *BaseRepository) Conn(ctx context.Context) Conn
```

`Conn(ctx)` vrátí `*sqlx.Tx` pokud běží transakce (z `TransactionMiddleware`), jinak `*sqlx.DB`.

### Implementace repozitáře

```go
// infrastructure/sqlite/user/repository.go

type Repository struct {
    sqlite.BaseRepository   // embed
}

func NewRepository(db *database.SqliteManager) *Repository {
    return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, u *user.User) error {
    const q = `INSERT INTO users (...) VALUES (...)`
    _, err := r.Conn(ctx).NamedExecContext(ctx, q, u)
    return err
}
```

Wire binduje doménový interface na konkrétní implementaci:

```go
wire.Bind(new(user.Repository), new(*sqliteuser.Repository))
```

### Seeder

`seeder.NewSeeder()` závisí na `user.Repository` (doménový interface) -- seeduje výchozí admin účet (nickname `admin`, heslo z **povinného a validovaného** `APP_SEED_ADMIN_PASSWORD`; žádné guessable default — seed bez nastaveného hesla selže). Seeder **neběží automaticky** -- spouští se ručně přes CLI `./bin/app seed`. Idempotentní: pokud uživatel `admin` existuje, nic nedělá. Pro vytvoření dalších uživatelů s libovolnou rolí slouží `./bin/app create-user`.

## Detaily

- SQLite je nastaven na WAL + `_txlock=immediate` + `busy_timeout(5000)` + `foreign_keys(on)` (viz `SqliteManager` výše).
- Repozitáře používají `sqlx` named queries (`NamedExecContext`) -- mapují struct fields přímo na SQL.
- Repozitáře volají `r.Conn(ctx)` -- nikdy přímo `r.DB.DB()`. Tím je zaručena transparentní účast v transakci. **Výjimka:** zápisy, které MUSÍ přežít rollback obklopující bus tx, vědomě jdou raw poolem (`r.DB.DB()`):
  - `user.Repository.RecordFailedLogin` / `ResetFailedLogin` -- bruteforce counter nesmí být shozen rollbackem failed-login handleru.
  - `audit.Repository.Save` -- security audit trail musí persistovat i u neúspěšných commandů (Audit middleware proto sedí mimo Transaction middleware).
  Žádné jiné repo legitimně raw pool nepoužívá; každá výjimka má komentář u metody s důvodem.
- Aktuální repozitáře: `sqlite/user/` (`user.Repository`), `sqlite/token/` (`token.TokenRepository`), `sqlite/job/` (`job.Repository`), `sqlite/audit/` (`shared.AuditLogger`), `sqlite/seeder/` (`shared.Seeder`).
- `sqlite/job/` má dvě precision-citlivá specifika u srovnání času: (1) SQL používá `julianday(...)` ne `strftime('%f', ...)` -- strftime na ms zaokrouhluje round-half-up, takže Go time s µs ≥ 500 by skončilo o ms napřed proti `'now'`; (2) `Enqueue`/`Reschedule` truncate `run_at` na `UTC + ms` přes `msPrecisionUTC` -- ncruces WASM `'now'` trailí Go `time.Now()` o ~1 ms a bez zarovnání na společnou precizi `ClaimDue` nahodile čerstvě enqueueovaný řádek nevidí.
