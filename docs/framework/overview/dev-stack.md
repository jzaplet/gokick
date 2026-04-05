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


## Proč

Single-binary Go server s embedovaným Vue 3 SPA. Po buildu vznikne jedna spustitelná binárka `./app serve`. Minimální verze Go 1.26.


## Jak

### Backend (Go)

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
| Hesla | `golang.org/x/crypto` | bcrypt (s SHA-256 prehash) |
| JWT | `github.com/golang-jwt/jwt/v5` | Generování a validace tokenů |
| Testování | `github.com/stretchr/testify` | Aserce a test suites |
| Arch linting | `github.com/fe3dback/go-arch-lint` | Kontrola závislostí mezi vrstvami |

### Frontend (Vue 3 + Vite)

| Komponenta | Knihovna | Účel |
|---|---|---|
| Framework | `vue@^3` | Reaktivní UI |
| Routing | `vue-router@^4` | Client-side SPA routing |
| Build tool | `vite` | Dev server + produkční build |
| CSS | `tailwindcss@^4` + `@tailwindcss/vite` | Utility-first styling |
| TypeScript | `typescript` + `vue-tsc` | Typová kontrola |
| Linting | `eslint` + `oxlint` | Statická analýza |


### Adresářová struktura

```
project/
├── cmd/main.go                       # Entry point
├── app/                              # Go backend
│   ├── application.go                # App lifecycle
│   │
│   ├── domain/                       # Vrstva 1: Čisté jádro
│   │   ├── shared/                   # AuthClaims, errors, events, interfaces
│   │   │                               (PasswordHasher, PermissionChecker, Transactor)
│   │   ├── user/                     # User entity, Nickname/Role VO, Repository interface
│   │   └── token/                    # RefreshToken entity, TokenRepository interface
│   │
│   ├── application/                  # Vrstva 2: Use cases
│   │   ├── bus/                      # CommandBus, QueryBus, EventBus
│   │   │   └── middleware/           # Recovery, logging, authorize, transaction, events
│   │   ├── command/                  # CQRS write operace
│   │   ├── query/                    # CQRS read operace
│   │   └── event/                    # Event handlery
│   │
│   ├── infrastructure/               # Vrstva 3: Implementace
│   │   ├── config/                   # Konfigurace (.env)
│   │   ├── database/                 # SqliteManager + MigrationManager
│   │   ├── sqlite/                   # BaseRepository, Conn, Seeder
│   │   │   ├── user/                 # user.Repository implementace
│   │   │   └── token/                # token.TokenRepository implementace
│   │   ├── security/                 # JWT, PasswordHasher, PermissionChecker
│   │   └── di/                       # Wire DI providers + wire_gen.go
│   │
│   └── presentation/                 # Vrstva 4: I/O
│       ├── http/
│       │   ├── handler/              # HTTP handlery
│       │   ├── middleware/           # HTTP middleware (CORS, JWT, logging)
│       │   ├── response/            # JSON response helpery
│       │   └── server/              # HTTP server + routing
│       └── console/                  # Cobra CLI
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


## Detaily

- SQLite je pure-Go (`ncruces/go-sqlite3`) -- žádné CGO, cross-compile bez problémů.
- `sqlx` používá `db:"..."` tagy na entity structech pro automatický struct scanning.
- `go-arch-lint` se spouští přes `make arch-check` a hlídá pravidla závislostí mezi vrstvami (viz [Architecture](/framework/overview/architecture)).
- Frontend se builduje do `public/` a embeduje se do Go binárky přes `embed.FS`.
