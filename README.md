---
layout: 'page'
uri: '/'
slug: 'home'
navTitle: 'Začínáme'
title: 'Go Skeleton'
description: 'Go CQRS DDD skeleton s Vue 3 SPA, SQLite databází a JWT autentizací – vše v jedné binárce.'
---

# Go Skeleton

Go **CQRS** (Command Query Responsibility Segregation) DDD skeleton s Vue 3 SPA, SQLite databází a JWT autentizací – vše v jedné binárce.


## Vlastnosti

- **Go** backend s **CQRS** command/query/event bus
- **DDD** architektura (domain → application → infrastructure → presentation)
- **Vue 3** SPA (Vite, TypeScript, Tailwind) embedovaná do Go binárky
- **SQLite** s migracemi (Goose)
- **JWT** access + refresh token autentizace
- **Wire** dependency injection
- **Striktní oddělení vrstev** přes interfaces – infrastrukturu (např. SQLite → Postgres) lze zaměnit bez zásahu do domény
- **go-arch-lint** vynucení architektonických pravidel


## Dokumentace

| Sekce | Popis |
|-------|-------|
| [Framework](/framework) | Architektura, vrstvy, infrastruktura |
| [Business logika](/business) | Specifikace obrazovek a business pravidel |
| [Codebase](/codebase) | Algoritmy a znovupoužitelné balíčky v rámci projektu |
