---
layout: 'page'
uri: '/framework/overview/layers'
position: 4
slug: 'framework-overview-layers'
parent: 'framework-overview'
navTitle: 'Layers'
title: 'Layers'
description: 'Přehled všech balíčků v jednotlivých vrstvách a jejich zodpovědnosti.'
---

# Layers


## Dependency matrix

Řádek smí importovat sloupec:

```
                domain  bus  busmw  cmd  qry  event  config  db  sqlite  sec  handler  httpmw  resp  server  console
domain            -      x    x     x    x     x      x      x    x      x     x        x      x      x       x
bus               Y      -    -     x    x     x      x      x    x      x     x        x      x      x       x
bus_middleware    Y      Y    -     x    x     x      x      x    x      x     x        x      x      x       x
command           Y      x    x     -    x     x      x      x    x      x     x        x      x      x       x
query             Y      x    x     x    -     x      x      x    x      x     x        x      x      x       x
event             Y      x    x     x    x     -      x      x    x      x     x        x      x      x       x
config            x      x    x     x    x     x      -      x    x      x     x        x      x      x       x
database          x      x    x     x    x     x      Y      -    x      x     x        x      x      x       x
sqlite            Y      x    x     x    x     x      x      Y    -      x     x        x      x      x       x
security          Y      x    x     x    x     x      Y      x    x      -     x        x      x      x       x
handler           Y      Y    x     Y    Y     x      x      x    x      x     -        x      Y      x       x
http_middleware   Y      x    x     x    x     x      x      x    x      Y     x        -      Y      x       x
response          x      x    x     x    x     x      x      x    x      x     x        x      -      x       x
server            x      x    x     x    x     x      Y      x    x      x     Y        Y      x      -       x
console           x      x    x     x    x     x      Y      Y    x      x     x        x      x      Y       -
```

Klíčová pravidla:
1. **Domain neimportuje nic** -- čisté jádro
2. **Command/Query závisí jen na domain** -- žádný bus, security, infra
3. **Handler neimportuje sqlite, security ani event**
4. **Bus middleware závisí na domain + bus**
5. **HTTP middleware závisí na security + response**
6. **DI smí vše** (excluded z arch-lintu)
7. **Response je izolovaný** -- žádné závislosti


## Domain

| Balíček | Co dělá |
|---|---|
| `domain/shared/` | Sdílené typy napříč kontexty -- error typy, service interfaces, auth context, eventy. |
| `domain/<context>/` | Entita, value objects, repository interface, domain events. Každý bounded context ve vlastním balíčku. |


## Application

| Balíček | Co dělá |
|---|---|
| `application/bus/` | Bus s middleware chain, generický dispatch, Wire wrapper typy. |
| `application/bus/middleware/` | Recovery, logging, autorizace, transakce, dispatch eventů. |
| `application/<domain>/command/` | Command structs + handlery. Write operace. Organizováno po doménách (`auth/`, `user/`, `profile/`, ...). |
| `application/<domain>/query/` | Query structs + handlery. Read operace. Organizováno po doménách. |
| `application/<domain>/event/` | Event handlery -- side-effects po úspěšném commitu. Organizováno po doménách. |


## Infrastructure

| Balíček | Co dělá |
|---|---|
| `infrastructure/config/` | Konfigurace z `.env`. |
| `infrastructure/database/` | Správa DB připojení, transakce, migrace. |
| `infrastructure/sqlite/` | Base repository s transparentní podporou transakcí, seeder. |
| `infrastructure/sqlite/<context>/` | Implementace doménových repository interfaces. Per-kontext balíček. |
| `infrastructure/security/` | Hashování hesel, JWT tokeny, kontrola oprávnění. |
| `infrastructure/di/` | Wire compile-time DI. Propojuje vše dohromady. |


## Presentation

| Balíček | Co dělá |
|---|---|
| `presentation/http/handler/` | HTTP handlery -- deserializace, bus dispatch, response. |
| `presentation/http/middleware/` | Trace ID, security headers (CSP/HSTS/…), CORS, logging, JWT auth. |
| `presentation/http/response/` | JSON response helpers, error → HTTP status mapování. |
| `presentation/http/server/` | Routing, middleware chain, SPA fallback. |
| `presentation/console/` | Cobra CLI příkazy. |
