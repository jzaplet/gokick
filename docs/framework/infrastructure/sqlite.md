---
layout: 'page'
uri: '/framework/infrastructure/sqlite'
position: 3
slug: 'framework-infrastructure-sqlite'
parent: 'framework-infrastructure'
navTitle: 'SQLite Repositories'
title: 'SQLite Repositories'
description: 'Balíček infrastructure/sqlite/ – implementace doménových repository interfaces.'
---

# SQLite Repositories

Balíček `infrastructure/sqlite/`. Implementuje doménové repository interfaces přes `sqlx`.

| Soubor | Implementuje |
|---|---|
| `sqlite_user_repository.go` | `user.Repository` |
| `sqlite_token_repository.go` | `token.TokenRepository` |


## Transakce

Repozitáře používají `database.TxFromContext(ctx)` pro práci v rámci transakce z bus `TransactionMiddleware`. Pokud transakce neexistuje v contextu, používají běžné spojení.
