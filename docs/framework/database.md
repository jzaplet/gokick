---
layout: 'page'
uri: '/framework/database'
position: 3
slug: 'framework-database'
parent: 'framework'
navTitle: 'Databáze'
title: 'Databáze'
description: 'SQLite databáze – driver, connection manager, Goose migrace.'
---

# Databáze


## Driver

Pure-Go SQLite driver `ncruces/go-sqlite3` – bez závislosti na CGO. Zjednodušuje cross-kompilaci a deployment.

Connection manager v `database/sqlite_manager.go` poskytuje `*sqlx.DB` instanci.


## Migrace (Goose)

Migrační soubory v `migrations/` jsou embedované do binárky přes `embed.go`. Spouští se automaticky při startu serveru.


### Skeleton migrace

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

```sql
-- migrations/20260327000002_create_refresh_tokens_table.sql

-- +goose Up
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

-- +goose Down
DROP TABLE IF EXISTS refresh_tokens;
```


### Správa migrací

```bash
make migrate-create NAME=create_users_table  # Nový soubor
make migrate-up                               # Aplikuj
make migrate-down                             # Rollback
make migrate-status                           # Stav
```


## Transakce přes context

`database.ContextWithTx` / `database.TxFromContext` umožňuje předat transakci přes `context.Context`. Používá ho `TransactionMiddleware` v bus balíčku – repozitáře pak automaticky pracují v rámci transakce.

Pro paralelní operace v rámci transakčního kontextu lze využít `sync.WaitGroup.Go()` (Go 1.25) pro čistší spouštění goroutin bez explicitního `Add`/`Done`.
