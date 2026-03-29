---
layout: 'page'
uri: '/framework/adapters/sqlite'
position: 4
slug: 'framework-adapters-sqlite'
parent: 'framework-adapters'
navTitle: 'SQLite Repozitáře'
title: 'SQLite Repozitáře'
description: 'Balíček sqlite/ – implementace doménových repository interfaces.'
---

# SQLite Repozitáře

Balíček `sqlite/`. Implementuje doménové repository interfaces přes `sqlx`.

| Soubor | Implementuje |
|---|---|
| `user_repository.go` | `domain.UserRepository` |
| `token_repository.go` | `domain.TokenRepository` |


## Transakce

Repozitáře používají `database.TxFromContext(ctx)` pro práci v rámci transakce z bus `TransactionMiddleware`. Pokud transakce neexistuje v contextu, používají běžné spojení.
