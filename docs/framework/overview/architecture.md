---
layout: 'page'
uri: '/framework/overview/architecture'
position: 2
slug: 'framework-overview-architecture'
parent: 'framework-overview'
navTitle: 'Architecture'
title: 'Architecture'
description: 'DDD vrstvy – domain, application, infrastructure, presentation.'
---

# Architecture

DDD s CQRS a bus pattern. Čtyři vrstvy s přísnými pravidly závislostí, komunikace přes CommandBus/QueryBus/EventBus.

| Vrstva | Složka | Balíčky | Popis |
|---|---|---|---|
| **Domain** | `domain/` | `shared/`, `user/`, `token/` | Entity, interfaces. Žádné závislosti. |
| **Application** | `application/` | `bus/`, `command/`, `query/`, `event/` | CQRS handlery, bus middleware. |
| **Infrastructure** | `infrastructure/` | `config/`, `database/`, `sqlite/`, `security/`, `di/` | Implementace interfaces, podpůrná infra. |
| **Presentation** | `presentation/` | `http/handler/`, `http/middleware/`, `http/server/`, `http/response/`, `console/` | HTTP a CLI vrstva. |


## Detaily per vrstva

- [Domain](/framework/domain) – entity, value objects, interfaces, error typy, eventy
- [Application](/framework/application) – commands, queries, bus, event handlery
- [Infrastruktura](/framework/infrastructure) – konfigurace, databáze, repozitáře, security, Wire DI
- [Prezentace](/framework/presentation) – HTTP server, handlery, middleware, response, CLI


## Cross-cutting

- [Authentication](/auth) – JWT access + refresh token
- [Frontend](/frontend) – Vue 3 SPA
- [Pravidla závislostí](/framework/overview/architecture-rules) – dependency matrix, go-arch-lint
- [Cross-domain izolace](/framework/overview/cross-domain) – bounded contexts, komunikace přes bus
- [Observability](/framework/infrastructure/observability) – trace ID, slog, Sentry
