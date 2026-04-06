---
layout: 'page'
uri: '/'
slug: 'home'
navTitle: 'Začínáme'
title: 'Go Skeleton'
description: 'Go CQRS DDD skeleton s Vue 3 SPA, SQLite databází a JWT autentizací – vše v jedné binárce.'
---

# Go Skeleton

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


## Dokumentace

| Sekce | Popis |
|-------|-------|
| [Framework](/framework) | Architektura, vrstvy, infrastruktura |
| [Business logika](/business) | Specifikace obrazovek a business pravidel |
| [Codebase](/codebase) | Algoritmy a znovupoužitelné balíčky v rámci projektu |
