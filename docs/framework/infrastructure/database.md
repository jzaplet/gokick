---
layout: 'page'
uri: '/framework/infrastructure/database'
position: 2
slug: 'framework-infrastructure-database'
parent: 'framework-infrastructure'
navTitle: 'Databáze'
title: 'Databáze'
description: 'Balíček database/ – SQLite manager, Goose migrace.'
---

# Databáze

Balíček `database/`. Pure-Go SQLite driver `ncruces/go-sqlite3` (bez CGO).


## SqliteManager

Poskytuje `*sqlx.DB` instanci. Transakční context: `ContextWithTx` / `TxFromContext`.


## MigrationManager

Goose migrace embedované v binárce. Spouští se automaticky při startu.


## Migrace

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
