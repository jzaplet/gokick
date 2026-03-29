---
layout: 'page'
uri: '/framework/overview/architecture'
position: 2
slug: 'framework-overview-architecture'
parent: 'framework-overview'
navTitle: 'Architektura'
title: 'Architektura'
description: 'Pragmatický hybrid – vrstvy, balíčky, závislosti.'
---

# Architektura

Pragmatický hybrid s CQRS a bus pattern. Flat balíčky, doménové interfaces, komunikace přes CommandBus/QueryBus/EventBus.

| Vrstva | Balíčky | Popis |
|---|---|---|
| **Domain** | `domain/` | Entity, interfaces. Žádné závislosti. |
| **Application** | `command/`, `query/`, `event/` | CQRS handlery. Závisí jen na domain. |
| **Bus** | `bus/` | Middleware chain (authorize, transaction, events). |
| **Adapters** | `handler/`, `middleware/`, `sqlite/`, `security/`, `response/` | Implementace interfaces. |
| **Infrastruktura** | `env/`, `database/`, `console/`, `server/`, `di_container/` | Podpůrná infrastruktura. |


## Detaily per vrstva

- [Domain](/framework/domain) – entity, value objects, interfaces, error typy, eventy
- [Application](/framework/application) – commands, queries, bus, event handlery
- [Adaptery](/framework/adapters) – HTTP server, handlery, SQLite, security, response
- [Infrastruktura](/framework/infrastructure) – konfigurace, databáze, CLI, Wire DI, build


## Cross-cutting

- [Autentizace](/framework/auth) – JWT access + refresh token
- [Frontend](/framework/frontend) – Vue 3 SPA
- [Pravidla závislostí](/framework/overview/architecture-rules) – dependency matrix, go-arch-lint
- [Cross-domain izolace](/framework/overview/cross-domain) – bounded contexts, komunikace přes bus
- [Observability](/framework/infrastructure/observability) – trace ID, slog, Sentry
