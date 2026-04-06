---
layout: 'page'
uri: '/'
slug: 'home'
navTitle: 'Getting Started'
title: 'GO Kick Docs'
description: 'GO CQRS DDD skeleton s Vue 3 SPA, SQLite databází a JWT autentizací – vše v jedné binárce.'
---

# GO Kick Docs

Go **DDD** (Domain-Driven Design) skeleton s **CQRS** (Command Query Responsibility Segregation), Vue 3 SPA, SQLite databází a JWT autentizací – vše v jedné binárce.


## Vlastnosti

- **DDD** – čtyřvrstvá architektura (domain → application → infrastructure → presentation) s bounded kontexty, entitami, value objects a domain eventy
- **CQRS** – oddělené command/query/event busy s middleware chain (logging, autorizace, transakce, recovery)
- **Dependency inversion** – doména definuje interfaces (porty), infrastruktura dodává implementace (adaptery). Př: SQLite lze zaměnit za Postgres bez zásahu do domény
- **Vue 3** SPA (Vite, TypeScript, Tailwind) embedovaná do Go binárky
- **SQLite** s migracemi (Goose), pure-Go bez CGO
- **JWT** access + refresh token autentizace
- **Wire** compile-time dependency injection
- **go-arch-lint** vynucení závislostí mezi vrstvami


## Rychlý start

```bash
corepack enable
cp .env.example .env
make install
make build && make serve
```

Server běží na `http://localhost:3000`. Podrobnosti v [Installation](/framework/overview/commands).


## Hlavní příkazy

| Příkaz | Co dělá |
|---|---|
| `make build` | Sestaví frontend + backend → `bin/app` |
| `make serve` | Spustí server |
| `make test` | Vitest + go test |
| `make lint` | ESLint + vue-tsc + golangci-lint + go-arch-lint |
| `make format` | ESLint Stylistic + golines |


## Dokumentace

| Sekce | Popis                                                |
|-------|------------------------------------------------------|
| [Framework](/framework) | Architektura, vrstvy, infrastruktura                 |
| [Guides](/guides) | Praktické návody — autentizace, frontend utility     |
| [Business Logic](/business) | Specifikace obrazovek a business pravidel projektu   |
| [Codebase](/codebase) | Algoritmy a znovupoužitelné balíčky v rámci projektu |
