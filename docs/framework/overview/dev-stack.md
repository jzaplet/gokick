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

Single-binary Go server s embedovaným Vue 3 SPA. Po buildu vznikne jedna spustitelná binárka `./app serve`. Minimální verze Go 1.26.

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
| Hesla | `golang.org/x/crypto` | bcrypt (s SHA-256 prehash) |
| JWT | `github.com/golang-jwt/jwt/v5` | Generování a validace tokenů |
| Arch linting | `github.com/fe3dback/go-arch-lint` | Kontrola závislostí mezi vrstvami |
| Error tracking | `github.com/getsentry/sentry-go` | Sentry — paniky + terminální selhání jobů (gated na DSN) |

## Frontend (Vue 3 + Vite)

| Komponenta | Knihovna | Účel |
|---|---|---|
| Framework | `vue@^3.5` | Reaktivní UI |
| Routing | `vue-router@^5` | Client-side SPA routing |
| Build tool | `vite@^8` | Dev server + produkční build |
| CSS | `tailwindcss@^4` + `@tailwindcss/vite` | Utility-first styling |
| TypeScript | `typescript@^6` + `vue-tsc` | Typová kontrola (maximum strictness) |
| Linting | `eslint@^10` + `typescript-eslint` + `eslint-plugin-vue` | Statická analýza (strictTypeChecked) |
| Formatting | `@stylistic/eslint-plugin` | Formátování kódu (nahrazuje Prettier) |
| Testování | `vitest@^4` + `@vue/test-utils` + `jsdom` | Unit testy komponent |
| Error tracking | `@sentry/vue@^10` | Sentry — Vue chyby + unhandled rejections (gated na DSN) |
| Source maps | `@sentry/vite-plugin@^5` | Upload source-map do Sentry (čitelné stack traces, dev-dependency) |
| Package manager | `yarn@4` (Berry, nodeLinker: node-modules) | Správa závislostí |


## Adresářová struktura

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
│   ├── application/                  # Vrstva 2: Use cases (po doménách)
│   │   ├── bus/                      # CommandBus, QueryBus, EventBus
│   │   │   └── middleware/           # Recovery, logging, authorize, transaction, events
│   │   ├── auth/                     # command/ (login, refresh, logout)
│   │   ├── profile/                  # command/ (change_password), query/ (get_profile)
│   │   ├── user/                     # command/ (create, update, delete), query/ (list)
│   │   └── dashboard/                # query/ (user_dashboard, admin_dashboard) -- placeholdery
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
│       │   ├── middleware/           # Trace, security headers, CORS, logging, JWT
│       │   ├── response/             # JSON response helpery
│       │   └── server/               # HTTP server + routing
│       └── console/                  # Cobra CLI (serve, seed, create-user)
│
├── assets/                           # Frontend zdrojáky (Vue 3 + Vite)
│   ├── app.ts                        # Vue mount point (createApp + router + CSS)
│   ├── App.vue                       # Root komponenta (<RouterView /> + Toast)
│   ├── router.ts                     # Vue Router (routes, guards)
│   ├── tailwind.css                  # Tailwind entry (@import 'tailwindcss')
│   ├── app/                          # Aplikační komponenty (po doménách)
│   │   ├── <Domain>/Views/           # Routované views (orchestrátory)
│   │   ├── <Domain>/Components/      # Doménové komponenty (formuláře, tabulky, karty)
│   │   ├── <Domain>/types/           # Typové definice (.ts soubory, jeden typ na soubor)
│   │   └── Layout/                   # AppLayout + AppHeader (chrome pro autentizovaný stav)
│   └── app-ui/                       # Sdílené, generic UI komponenty a composables
│       ├── Auth/                     # useAuth, authFetch, state, login/logout/refresh, permissions + typy
│       ├── Fetch/                    # apiFetch, apiUpload, apiDownload, accessToken, parseResponse + typy
│       ├── Alerts/                   # ErrorAlert
│       ├── Buttons/                  # Button
│       ├── ClickOutside/             # useClickOutside composable
│       ├── Dropdown/                 # Dropdown (slot-based, click-outside auto-close)
│       ├── Icons/                    # SVG ikony
│       ├── Inputs/                   # Input, Select, CheckBox, DateTimeInput
│       ├── Loading/                  # Spinner
│       ├── Modals/                   # Modal, ConfirmModal
│       └── Toast/                    # useToast + Toast komponenty
│
├── tests/                            # Frontend testy (Vitest + Vue Test Utils)
│
├── public/                           # Vite build output (embedováno do Go binárky)
│   ├── embed.go                      # //go:embed * → embed.FS
│   ├── favicon.ico                   # Favicon (commitováno)
│   ├── index.html                    # Generovaný Vite entry (gitignored)
│   └── assets/                       # Generované JS/CSS bundly (gitignored)
├── migrations/                       # Goose SQL migrace (embed)
├── docs/                             # Documan dokumentace (markdown)
├── docker/                           # Docker konfigurace
│   ├── production/                    # Produkční Dockerfile (multi-stage)
│   └── documan/                       # Documan dev service
│
├── Makefile                          # Build, lint, format, migrate, serve
├── go.mod / go.sum                   # Go dependencies
├── package.json / yarn.lock          # Frontend dependencies
├── .yarnrc.yml                       # Yarn v4 konfigurace (nodeLinker: node-modules)
├── vite.config.ts                    # Vite build + dev proxy konfigurace
├── tsconfig.json                     # TypeScript (maximum strictness)
├── eslint.config.ts                  # ESLint (strictTypeChecked + Stylistic)
├── index.html                        # Vite HTML entry point
├── env.d.ts                          # TypeScript deklarace (Vite client, .vue moduly)
├── .env / .env.example               # Env konfigurace (porty, DB, JWT)
├── .go-arch-lint.yml                 # Pravidla závislostí mezi vrstvami
└── docker-compose.yml                # Docker services (documan)
```


## Detaily

- SQLite je pure-Go (`ncruces/go-sqlite3`) -- žádné CGO, cross-compile bez problémů.
- `sqlx` používá `db:"..."` tagy na entity structech pro automatický struct scanning.
- `go-arch-lint` se spouští přes `make arch-check` a hlídá pravidla závislostí mezi vrstvami (viz [Architecture](/framework/overview/architecture)).
- Frontend se builduje do `public/` a embeduje se do Go binárky přes `embed.FS`.
- Error tracking přes **Sentry** (BE `sentry-go` i FE `@sentry/vue`) je gated na DSN — bez `APP_SENTRY_DSN` / `APP_SENTRY_DSN_FRONTEND` běží jako no-op. Hlásí se jen neočekávané chyby (recovered paniky, terminální selhání jobů, Vue chyby + rejections), ne běžné 4xx. Detaily: [Observability](/framework/infrastructure/observability), nastavení: [Sentry guide](/guides/sentry).
