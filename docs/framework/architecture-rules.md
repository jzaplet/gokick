---
layout: 'page'
uri: '/framework/architecture-rules'
position: 11
slug: 'framework-architecture-rules'
parent: 'framework'
navTitle: 'Pravidla závislostí'
title: 'Pravidla závislostí'
description: 'Dependency matrix, go-arch-lint enforcement, klíčová pravidla.'
---

# Pravidla závislostí


## Dependency matrix

Řádek smí importovat sloupec.

```
                  domain  cmd  qry  bus  sqlite  sec  handler  mw   server  console  di   env  db   resp
domain              -      ✗    ✗    ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     ✗
command             ✓      -    ✗    ✗     ✗      ✓     ✗      ✗      ✗       ✗      ✗    ✗    ✗     ✗
query               ✓      ✗    -    ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     ✗
bus                 ✗      ✗    ✗    -     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✓     ✗
sqlite              ✓      ✗    ✗    ✗     -      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✓     ✗
security            ✓      ✗    ✗    ✗     ✗      -     ✗      ✗      ✗       ✗      ✗    ✓    ✗     ✗
handler             ✓      ✓    ✓    ✓     ✗      ✓     -      ✗      ✗       ✗      ✗    ✗    ✗     ✓
middleware          ✓      ✗    ✗    ✗     ✗      ✓     ✗      -      ✗       ✗      ✗    ✗    ✗     ✓
server              ✗      ✗    ✗    ✗     ✗      ✗     ✓      ✓      -       ✗      ✗    ✓    ✗     ✗
console             ✗      ✗    ✗    ✗     ✗      ✗     ✗      ✗      ✓       -      ✗    ✓    ✓     ✗
di_container        ✓      ✓    ✓    ✓     ✓      ✓     ✓      ✓      ✓       ✓      -    ✓    ✓     ✓
env                 ✗      ✗    ✗    ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    -    ✗     ✗
database            ✗      ✗    ✗    ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✓    -     ✗
response            ✗      ✗    ✗    ✗     ✗      ✗     ✗      ✗      ✗       ✗      ✗    ✗    ✗     -
```

**Legenda:** `✓` smí, `✗` nesmí, `-` sám sebe


## Klíčová pravidla

1. **Domain neimportuje nic** – čisté jádro bez závislostí
2. **Command/Query závisí jen na domain** (command + security pro bcrypt)
3. **Command/Query neznají bus** – bus je volán z handler vrstvy
4. **Handler nikdy neimportuje sqlite** – pracuje přes bus → CQRS → domain interfaces
5. **Bus závisí jen na database** (TransactionMiddleware) a stdlib
6. **Di_container smí vše** – jeho role je propojení
7. **Response je izolovaný** – jen stdlib


## Enforcement – go-arch-lint

```bash
go install github.com/fe3dback/go-arch-lint@latest
```

Konfigurace `.go-arch-lint.yml`:

```yaml
version: 3
workdir: app

components:
  domain:
    in: domain/**
  command:
    in: command/**
  query:
    in: query/**
  bus:
    in: bus/**
  sqlite:
    in: sqlite/**
  security:
    in: security/**
  handler:
    in: handler/**
  middleware:
    in: middleware/**
  server:
    in: server/**
  console:
    in: console/**
  di_container:
    in: di_container/**
  env:
    in: env/**
  database:
    in: database/**
  response:
    in: response/**

deps:
  domain:
    mayDependOn: []
  command:
    mayDependOn: [domain, security]
  query:
    mayDependOn: [domain]
  bus:
    mayDependOn: [database]
  sqlite:
    mayDependOn: [domain, database]
  security:
    mayDependOn: [domain, env]
  handler:
    mayDependOn: [domain, command, query, bus, security, response]
  middleware:
    mayDependOn: [domain, security, response]
  server:
    mayDependOn: [handler, middleware, env]
  console:
    mayDependOn: [server, env, database]
  di_container:
    mayDependOn: [domain, command, query, bus, sqlite, security, handler, middleware, server, console, env, database, response]
  env:
    mayDependOn: []
  database:
    mayDependOn: [env]
  response:
    mayDependOn: []
```

```bash
make arch-check       # Kontrola pravidel
go-arch-lint graph    # Vizualizace závislostí
```


## Přidání nové feature

1. Rozšiř `domain/` – entity, value objects (se vstupní validací), repository interfaces
2. Implementuj repository metody v `sqlite/`
3. Vytvoř command/query v `command/` nebo `query/` – command handler validuje přes value objects a business pravidla
4. Rozšiř HTTP handler v `handler/` – přes bus, mapuje `ValidationError` na 400
5. Zaregistruj route v `server/`
6. Přidej Wire provider v `di_container/`
7. `make di && make arch-check`
