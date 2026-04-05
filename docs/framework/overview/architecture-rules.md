---
layout: 'page'
uri: '/framework/overview/architecture-rules'
position: 4
slug: 'framework-overview-architecture-rules'
parent: 'framework-overview'
navTitle: 'Dependency Rules'
title: 'Dependency Rules'
description: 'Dependency matrix, go-arch-lint, klíčová pravidla.'
---

# Dependency Rules


## Vrstvy

```
presentation → application → domain ← infrastructure
     │                                      ↑
     └──────────────────────────────────────┘
```

- **Domain** neimportuje nic – čisté jádro
- **Application** závisí jen na domain (přes bus + CQRS handlery)
- **Infrastructure** závisí na domain (implementuje interfaces)
- **Presentation** závisí na application + infrastructure (volá bus, čte config)
- **DI** smí vše (excluded z arch-lintu)


## Dependency matrix

Řádek smí importovat sloupec.

```
                domain  bus  busmw  cmd  qry  event  config  db  sqlite  sec  handler  httpmw  resp  server  console
domain            -      ✗    ✗     ✗    ✗     ✗      ✗      ✗    ✗      ✗     ✗        ✗      ✗      ✗       ✗
bus               ✓      -    –     ✗    ✗     ✗      ✗      ✗    ✗      ✗     ✗        ✗      ✗      ✗       ✗
bus_middleware    ✓      ✓    -     ✗    ✗     ✗      ✗      ✗    ✗      ✗     ✗        ✗      ✗      ✗       ✗
command           ✓      ✗    ✗     -    ✗     ✗      ✗      ✗    ✗      ✗     ✗        ✗      ✗      ✗       ✗
query             ✓      ✗    ✗     ✗    -     ✗      ✗      ✗    ✗      ✗     ✗        ✗      ✗      ✗       ✗
event             ✓      ✗    ✗     ✗    ✗     -      ✗      ✗    ✗      ✗     ✗        ✗      ✗      ✗       ✗
config            ✗      ✗    ✗     ✗    ✗     ✗      -      ✗    ✗      ✗     ✗        ✗      ✗      ✗       ✗
database          ✗      ✗    ✗     ✗    ✗     ✗      ✓      -    ✗      ✗     ✗        ✗      ✗      ✗       ✗
sqlite            ✓      ✗    ✗     ✗    ✗     ✗      ✗      ✓    -      ✗     ✗        ✗      ✗      ✗       ✗
security          ✓      ✗    ✗     ✗    ✗     ✗      ✓      ✗    ✗      -     ✗        ✗      ✗      ✗       ✗
handler           ✓      ✓    ✗     ✓    ✓     ✗      ✗      ✗    ✗      ✗     -        ✗      ✓      ✗       ✗
http_middleware   ✓      ✗    ✗     ✗    ✗     ✗      ✗      ✗    ✗      ✓     ✗        -      ✓      ✗       ✗
response          ✗      ✗    ✗     ✗    ✗     ✗      ✗      ✗    ✗      ✗     ✗        ✗      -      ✗       ✗
server            ✗      ✗    ✗     ✗    ✗     ✗      ✓      ✗    ✗      ✗     ✓        ✓      ✗      -       ✗
console           ✗      ✗    ✗     ✗    ✗     ✗      ✓      ✓    ✗      ✗     ✗        ✗      ✗      ✓       -
```


## Klíčová pravidla

1. **Domain neimportuje nic** – čisté jádro
2. **Command/Query závisí jen na domain**
3. **Command/Query neznají bus ani security**
4. **Handler neimportuje sqlite, security ani event**
5. **Bus middleware závisí na domain + bus**
6. **HTTP middleware závisí na security** (JWT validace) + response
7. **DI smí vše** (excluded z arch-lintu)
8. **Response je izolovaný** – žádné závislosti


## go-arch-lint

```bash
go install github.com/fe3dback/go-arch-lint@latest
make arch-check
```

Konfigurace `.go-arch-lint.yml` v kořeni projektu – viz [Dev Stack](/framework/overview/dev-stack).


## Přidání nové feature

1. `domain/` – entity, value objects, interfaces
2. `infrastructure/sqlite/` – repository implementace
3. `application/command/` nebo `application/query/` – CQRS handler s `Permissioned` nebo `SkipPermission`
4. `presentation/http/handler/` – HTTP handler přes bus
5. `presentation/http/server/` – registrace route
6. `infrastructure/di/` – Wire provider
7. `make di && make arch-check`
