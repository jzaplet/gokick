---
layout: 'page'
uri: '/framework/overview/architecture-rules'
position: 4
slug: 'framework-overview-architecture-rules'
parent: 'framework-overview'
navTitle: 'Pravidla závislostí'
title: 'Pravidla závislostí'
description: 'Dependency matrix, go-arch-lint, klíčová pravidla.'
---

# Pravidla závislostí


## Dependency matrix

Řádek smí importovat sloupec.

```
                  domain  cmd  qry  event  bus  sqlite  sec  handler  mw   server  console  di   env  db   resp
domain              -      ✗    ✗    ✗      ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     ✗
command             ✓      -    ✗    ✗      ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     ✗
query               ✓      ✗    -    ✗      ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     ✗
event               ✓      ✗    ✗    -      ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     ✗
bus                 ✓      ✗    ✗    ✗      -     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✓     ✗
sqlite              ✓      ✗    ✗    ✗      ✗     -      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✓     ✗
security            ✓      ✗    ✗    ✗      ✗     ✗      -     ✗      ✗      ✗       ✗      ✗    ✓    ✗     ✗
handler             ✓      ✓    ✓    ✗      ✓     ✗      ✗     -      ✗      ✗       ✗      ✗    ✗    ✗     ✓
middleware          ✓      ✗    ✗    ✗      ✗     ✗      ✓     ✗      -      ✗       ✗      ✗    ✗    ✗     ✓
server              ✗      ✗    ✗    ✗      ✗     ✗      ✗     ✓      ✓      -       ✗      ✗    ✓    ✗     ✗
console             ✗      ✗    ✗    ✗      ✗     ✗      ✗     ✗      ✗      ✓       -      ✗    ✓    ✓     ✗
di_container        ✓      ✓    ✓    ✓      ✓     ✓      ✓     ✓      ✓      ✓       ✓      -    ✓    ✓     ✓
env                 ✗      ✗    ✗    ✗      ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    -    ✗     ✗
database            ✗      ✗    ✗    ✗      ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✓    -     ✗
response            ✗      ✗    ✗    ✗      ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     -
```


## Klíčová pravidla

1. **Domain neimportuje nic** – čisté jádro
2. **Command/Query závisí jen na domain**
3. **Command/Query neznají bus ani security**
4. **Handler neimportuje sqlite, security ani event**
5. **Bus závisí na domain + database**
6. **Middleware závisí na security** (JWT validace)
7. **Di_container smí vše**
8. **Response je izolovaný**


## go-arch-lint

```bash
go install github.com/fe3dback/go-arch-lint@latest
make arch-check
```

Konfigurace `.go-arch-lint.yml` v kořeni projektu – viz [Dev Stack](/framework/overview/dev-stack).


## Přidání nové feature

1. `domain/` – entity, value objects, interfaces
2. `sqlite/` – repository implementace
3. `command/` nebo `query/` – CQRS handler s `Permissioned` nebo `SkipPermission`
4. `handler/` – HTTP handler přes bus
5. `server/` – registrace route
6. `di_container/` – Wire provider
7. `make di && make arch-check`
