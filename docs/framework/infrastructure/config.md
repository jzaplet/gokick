---
layout: 'page'
uri: '/framework/infrastructure/config'
position: 1
slug: 'framework-infrastructure-config'
parent: 'framework-infrastructure'
navTitle: 'Config'
title: 'Config'
description: 'Balíček infrastructure/config/ -- .env soubory, Config struct.'
---

# Config

## Proč

Centrální konfigurace aplikace z `.env` souboru. Jedna struktura, žádné globální proměnné. `LoadConfig()` načte soubor přes `godotenv` a vrátí `*Config` s naparsovanými hodnotami.

## Jak

### .env soubor

```env
APP_HTTP_PORT=3000
APP_DB_PATH=./data/app.db
APP_JWT_SECRET=min-32-chars-random-secret-key-here
APP_JWT_ACCESS_EXPIRATION=15m
APP_JWT_REFRESH_EXPIRATION=168h
APP_CORS_ORIGIN=http://localhost:5173
APP_COOKIE_SECURE=false
```

### Config struct

```go
// infrastructure/config/config.go

type Config struct {
    HTTPPort             string
    DBPath               string
    DBJournalMode        string
    JWTSecret            string
    JWTAccessExpiration  time.Duration
    JWTRefreshExpiration time.Duration
    CORSOrigin           string
    CookieSecure         bool
    SeedAdminPassword    string
    TrustProxyHeaders    bool
    RateLimitLogin       string
    RateLimitRefresh     string
}

func LoadConfig() (*Config, error)
```

## Detaily

| Proměnná | Default | Popis |
|---|---|---|
| `APP_HTTP_PORT` | `3000` | Port HTTP serveru |
| `APP_DB_PATH` | `./data/app.db` | Cesta k SQLite databázi |
| `APP_DB_JOURNAL_MODE` | `WAL` | SQLite journal mode -- whitelist `WAL`\|`DELETE`\|`MEMORY` |
| `APP_JWT_SECRET` | -- | JWT podpisový klíč (min. 32 znaků) |
| `APP_JWT_ACCESS_EXPIRATION` | `15m` | Životnost access tokenu |
| `APP_JWT_REFRESH_EXPIRATION` | `168h` | Životnost refresh tokenu |
| `APP_CORS_ORIGIN` | `http://localhost:5173` | Povolený CORS origin |
| `APP_COOKIE_SECURE` | `true` | Posílat refresh cookie jen přes HTTPS (viz níže) |
| `APP_SEED_ADMIN_PASSWORD` | -- | Heslo admina pro `./bin/app seed` (povinné jen pro seed, 8--128 znaků) |
| `APP_TRUST_PROXY_HEADERS` | `false` | Číst IP z `X-Real-IP` (zapnout jen za důvěryhodnou reverse proxy) |
| `APP_RATE_LIMIT_LOGIN` | `10/min` | Per-IP limit na `/auth/login` (prázdné = vypnuto) |
| `APP_RATE_LIMIT_REFRESH` | `60/min` | Per-IP limit na `/auth/refresh` (prázdné = vypnuto) |

> Config struct má **12 polí** (výše). `.env` snippet nahoře je jen ukázkový výřez; úplný seznam proměnných je v této tabulce a v `.env.example`.

- `APP_JWT_SECRET` je povinný a musí mít **min. 32 znaků** (HS256 floor). Validace ho odmítne při startu — provádí ji `NewJwtService` (security), ne `LoadConfig` (ta jen parsuje durations); chybějící/krátký secret tak shodí konstrukci aplikace přes Wire.
- Duration proměnné se parsují přes `time.ParseDuration` (jediné, co může `LoadConfig` selhat).
- Bool proměnné parsují řetězec `"true"` jako `true`, vše ostatní jako `false`.

### APP_COOKIE_SECURE

Řídí `Secure` flag na refresh cookie, který prohlížeč používá pro `/api/v1/auth/refresh`. Stejný flag zároveň gate-uje HSTS hlavičku v `SecurityHeadersMiddleware` -- `Strict-Transport-Security` se posílá jen v produkčním režimu.

- `true` (produkce, default) — prohlížeč pošle cookie **jen přes HTTPS**, server posílá HSTS. Nad HTTP se cookie neodešle, refresh selže.
- `false` (lokální vývoj) — cookie se posílá i přes plain HTTP, HSTS se nevysílá. Nutné pro vývoj na `http://localhost` (Vite dev server + Go backend jsou oba HTTP).

V `.env.example` je `false` kvůli dev workflow. V produkci **vždy** `true` + nasazení za TLS terminátor.

Ostatní flagy cookie jsou hardcoded, protože nemá smysl je měnit: `HttpOnly=true` (nepřístupné z JS, obrana proti XSS), `SameSite=Strict` (nepošle se při cross-site requestu, obrana proti CSRF), `Path=/api/v1/auth` (posílá se jen na auth endpointy).

### Documan

`DOCUMAN_HTTP_PORT` (default `3005` v `.env.example`) — port pro `documan` Docker service. `docker-compose.yml` ho interpoluje přes `${DOCUMAN_HTTP_PORT}`, nehardcoduje. Slouží jen pro lokální preview dokumentace, nesouvisí s aplikační binárkou.
