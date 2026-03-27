---
layout: 'page'
uri: '/codebase/dev-stack'
position: 1
slug: 'codebase-dev-stack'
parent: 'codebase'
navTitle: 'Dev Stack'
title: 'Dev Stack'
description: 'Přehled technologického stacku a implementačních fází.'
---

# Dev Stack

Single-binary Go server s embedovaným Vue 3 SPA. Minimální verze Go 1.26. Po buildu vznikne jedna spustitelná binárka: `./filmshes serve`.


## Technologický stack


### Backend (Go)

| Komponenta | Knihovna | Účel |
|---|---|---|
| CLI framework | `github.com/spf13/cobra` | Definice příkazů |
| Dependency Injection | `github.com/google/wire` | Compile-time DI |
| HTTP server | `net/http` (stdlib) | Go 1.26 routing (307 trailing slash redirecty) |
| Databáze | `github.com/ncruces/go-sqlite3` | Pure-Go SQLite (bez CGO) |
| SQL extensions | `github.com/jmoiron/sqlx` | Named queries, struct scanning |
| Migrace | `github.com/pressly/goose/v3` | Verzované SQL migrace |
| Env konfigurace | `github.com/joho/godotenv` | Načítání `.env` |
| UUID | `github.com/google/uuid` | Unikátní identifikátory |
| Hesla | `golang.org/x/crypto` | bcrypt |
| JWT | `github.com/golang-jwt/jwt/v5` | Generování a validace tokenů |
| Testování | `github.com/stretchr/testify` | Aserce a test suites |


### Frontend (Vue 3 + Vite)

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
filmshes/
├── app/                              # Go backend
│   ├── main.go                       # Entry point
│   ├── application.go                # App lifecycle
│   ├── domain/                       # Entity + interfaces
│   ├── command/                      # CQRS write operace
│   ├── query/                        # CQRS read operace
│   ├── bus/                          # CommandBus, QueryBus, middleware
│   ├── sqlite/                       # Repository implementace
│   ├── security/                     # JWT, bcrypt, auth context
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
├── public/                           # Vite build output (embed do binárky)
├── migrations/                       # Goose SQL migrace (embed)
├── docker/release/                   # Produkční Dockerfile
│
├── go.mod / package.json / Makefile
├── vite.config.ts / tsconfig.json
├── .env / .go-arch-lint.yml
└── docker-compose.yml
```

Podrobnosti ke každé vrstvě viz [Framework](/framework).


## Plán implementace


### Fáze 1: Základ Go + HTTP server

1. Go modul, Cobra CLI, Wire DI skeleton
2. `.env` konfigurace, HTTP server, health check
3. CORS, logging middleware, JSON response helper
4. Makefile: `install`, `dev`, `build`, `serve`, `di`


### Fáze 2: SQLite + migrace + bus

1. SQLite manager, Goose migration manager s embed
2. Migrace: `users`, `refresh_tokens`
3. Seed: výchozí admin uživatel
4. Bus: `Exec`, `ExecVoid` + middleware (logging, recovery, transaction)
5. Makefile: `migrate-*`


### Fáze 3: Auth (backend)

1. Domain: `User`, `RefreshToken`, repository interfaces
2. SQLite repozitáře, security služby (JWT, bcrypt)
3. HTTP middleware: JWT auth, role guard
4. CQRS: login, refresh, logout, profile
5. HTTP handlery přes bus, Wire propojení


### Fáze 4: User management (backend)

1. CQRS: create/update/delete user, change password
2. Admin HTTP handlery, user profil
3. Wire propojení


### Fáze 5: Frontend (Vue 3 SPA)

1. Vue 3 + Vite + TypeScript + Tailwind
2. Embedding do binárky, SPA fallback, Vite proxy
3. `useAuth` composable, `apiFetch`, route guards
4. Login screen, layout, navigace


### Fáze 6: Integrace + build

1. `make build` pipeline: di → fe-build → go-build
2. Cross-platform buildy


### Fáze 7: Docker + deployment

1. `Dockerfile.release`, `docker-compose.yml`
2. CI pipeline skeleton


### Fáze 8: Kvalita kódu

1. ESLint, golangci-lint, go-arch-lint
2. `go fix ./...` – automatická modernizace kódu na idiomatické Go 1.26 patterny
3. `make check` – kompletní CI pipeline
