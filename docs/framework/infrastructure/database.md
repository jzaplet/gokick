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

Při vytvoření nastavuje WAL mode a foreign keys. Implementuje `shared.Transactor` interface (duck typing) -- používá ho `TransactionMiddleware`.

Transakce v contextu:

```go
// database/sqlite_manager.go
func (m *SqliteManager) BeginTx(ctx context.Context) (context.Context, error)
func (m *SqliteManager) Commit(ctx context.Context) error
func (m *SqliteManager) Rollback(ctx context.Context) error
func TxFromContext(ctx context.Context) *sqlx.Tx
```

### Migrace (Goose)

`MigrationManager` embeduje SQL migrace z `/migrations/` do binárky a spouští je automaticky při startu.

```sql
-- migrations/20260327000001_create_users_table.sql

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

-- +goose Down
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

`sqlite.NewSeeder()` závisí na `user.Repository` (doménový interface) -- seeduje výchozí data při startu.

## Detaily

- SQLite je nastaven na WAL mode a foreign keys enabled.
- Repozitáře používají `sqlx` named queries (`NamedExecContext`) -- mapují struct fields přímo na SQL.
- Všechny repozitáře volají `r.Conn(ctx)` -- nikdy přímo `r.DB.DB()`. Tím je zaručena transparentní účast v transakci.
- Aktuální repozitáře: `sqlite/user/` (`user.Repository`), `sqlite/token/` (`token.TokenRepository`).
