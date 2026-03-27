---
layout: 'page'
uri: '/codebase/architecture'
position: 2
slug: 'codebase-architecture'
parent: 'codebase'
navTitle: 'Architektura'
title: 'Architektura'
description: 'Přehled architektury – vrstvy, tok requestu, odkazy na detaily.'
---

# Architektura

Pragmatický hybrid s CQRS a bus pattern. Flat balíčky, doménové interfaces, komunikace přes CommandBus/QueryBus.


## Vrstvy

```
┌──────────────────────────────────────────────────────────┐
│                     ENTRY POINTS                          │
│              console/, server/, di_container/              │
│                                                           │
│  ┌────────────────────────────────────────────────────┐   │
│  │                   ADAPTERS                          │   │
│  │      handler/, middleware/, sqlite/, security/      │   │
│  │                                                     │   │
│  │  ┌──────────────────────────────────────────────┐   │   │
│  │  │              APPLICATION                      │   │   │
│  │  │           command/, query/                     │   │   │
│  │  │                                                │   │   │
│  │  │  ┌────────────────────────────────────────┐   │   │   │
│  │  │  │              DOMAIN                     │   │   │   │
│  │  │  │            domain/                      │   │   │   │
│  │  │  └────────────────────────────────────────┘   │   │   │
│  │  └──────────────────────────────────────────────┘   │   │
│  └────────────────────────────────────────────────────┘   │
│                                                           │
│  ┌────────────────────────────────────────────────────┐   │
│  │                   BUS (cross-cutting)               │   │
│  │                      bus/                           │   │
│  └────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────┘
```

| Vrstva | Balíčky | Popis |
|---|---|---|
| **Domain** | `domain/` | Entity, interfaces. Žádné závislosti. |
| **Application** | `command/`, `query/` | CQRS handlery. Závisí jen na domain. |
| **Adapters** | `handler/`, `middleware/`, `sqlite/`, `security/` | Implementace interfaces. |
| **Entry Points** | `console/`, `server/`, `di_container/` | Propojení vrstev. |
| **Bus** | `bus/` | Middleware chain (logging, transakce, recovery). |
| **Podpůrné** | `env/`, `database/`, `response/` | Technická infrastruktura. |


## Tok requestu

```
HTTP Request
  → HTTP middleware (CORS, logging, JWT auth)
    → HTTP handler
      → bus.Exec / bus.ExecVoid
        → Bus middleware: Recovery → Logging → Transaction
          → Command/Query Handler
            → Repository interface (domain)
              → SQLite implementace
```


## Detaily

- [Domain](/framework/domain) – entity, repository interfaces
- [CQRS + Bus](/framework/cqrs) – commands, queries, bus middleware
- [Adaptery](/framework/adapters) – HTTP server, handlery, SQLite, security
- [Pravidla závislostí](/framework/architecture-rules) – dependency matrix, go-arch-lint
- [Wire DI](/framework/wire) – dependency injection
- [Autentizace](/framework/auth) – JWT access + refresh token
