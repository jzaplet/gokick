---
layout: 'page'
uri: '/framework/overview/dev-stack'
position: 1
slug: 'framework-overview-dev-stack'
parent: 'framework-overview'
navTitle: 'Dev Stack'
title: 'Dev Stack'
description: 'Technologický stack a adresářová struktura.'
---

# Dev Stack

Single-binary Go server s embedovaným Vue 3 SPA. Minimální verze Go 1.26. Po buildu vznikne jedna spustitelná binárka: `./app serve`.


## Backend (Go)

| Komponenta | Knihovna | Účel |
|---|---|---|
| CLI framework | `github.com/spf13/cobra` | Definice příkazů |
| Dependency Injection | `github.com/google/wire` | Compile-time DI |
| HTTP server | `net/http` (stdlib) | Go 1.26 routing |
| Databáze | `github.com/ncruces/go-sqlite3` | Pure-Go SQLite (bez CGO) |
| SQL extensions | `github.com/jmoiron/sqlx` | Named queries, struct scanning |
| Migrace | `github.com/pressly/goose/v3` | Verzované SQL migrace |
| Env konfigurace | `github.com/joho/godotenv` | Načítání `.env` |
| UUID | `github.com/google/uuid` | Unikátní identifikátory |
| Hesla | `golang.org/x/crypto` | bcrypt |
| JWT | `github.com/golang-jwt/jwt/v5` | Generování a validace tokenů |
| Testování | `github.com/stretchr/testify` | Aserce a test suites |


## Frontend (Vue 3 + Vite)

| Komponenta | Knihovna | Účel |
|---|---|---|
| Framework | `vue@^3` | Reaktivní UI |
| Routing | `vue-router@^4` | Client-side SPA routing |
| Build tool | `vite` | Dev server + produkční build |
| CSS | `tailwindcss@^4` + `@tailwindcss/vite` | Utility-first styling |
| TypeScript | `typescript` + `vue-tsc` | Typová kontrola |
| Linting | `eslint` + `oxlint` | Statická analýza |


## Adresářová struktura

```
project/
├── app/                              # Go backend
│   ├── main.go                       # Entry point
│   ├── application.go                # App lifecycle
│   ├── domain/                       # Entity, VO, interfaces, AuthClaims, events
│   ├── command/                      # CQRS write operace
│   ├── query/                        # CQRS read operace
│   ├── bus/                          # CommandBus, QueryBus, EventBus
│   ├── event/                        # Event handlery
│   ├── sqlite/                       # Repository implementace
│   ├── security/                     # JWT, bcrypt, permission checker
│   ├── handler/                      # HTTP handlery
│   ├── middleware/                   # HTTP middleware
│   ├── server/                       # HTTP server + routing
│   ├── database/                     # SQLite + migration manager
│   ├── env/                          # Konfigurace (.env)
│   ├── console/                      # Cobra CLI
│   ├── response/                     # JSON response helpery
│   └── di_container/                 # Wire DI
│
├── assets/                           # Frontend (Vue 3 + Vite)
├── public/                           # Vite build output (embed)
├── migrations/                       # Goose SQL migrace (embed)
├── docker/release/                   # Produkční Dockerfile
│
├── go.mod / package.json / Makefile
├── vite.config.ts / tsconfig.json
├── .env / .go-arch-lint.yml
└── docker-compose.yml
```
