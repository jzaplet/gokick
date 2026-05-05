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
    JWTSecret            string
    JWTAccessExpiration  time.Duration
    JWTRefreshExpiration time.Duration
    CORSOrigin           string
    CookieSecure         bool
}

func LoadConfig() (*Config, error)
```

## Detaily

| Proměnná | Default | Popis |
|---|---|---|
| `APP_HTTP_PORT` | `3000` | Port HTTP serveru |
| `APP_DB_PATH` | `./data/app.db` | Cesta k SQLite databázi |
| `APP_JWT_SECRET` | -- | JWT podpisový klíč (min. 32 znaků) |
| `APP_JWT_ACCESS_EXPIRATION` | `15m` | Životnost access tokenu |
| `APP_JWT_REFRESH_EXPIRATION` | `168h` | Životnost refresh tokenu |
| `APP_CORS_ORIGIN` | `http://localhost:5173` | Povolený CORS origin |
| `APP_COOKIE_SECURE` | `true` | Posílat refresh cookie jen přes HTTPS (viz níže) |

- `LoadConfig()` vrací error pokud `APP_JWT_SECRET` chybí -- je povinný.
- Duration proměnné se parsují přes `time.ParseDuration`.
- Bool proměnné parsují řetězec `"true"` jako `true`, vše ostatní jako `false`.

### APP_COOKIE_SECURE

Řídí `Secure` flag na refresh cookie, který prohlížeč používá pro `/api/v1/auth/refresh`. Stejný flag zároveň gate-uje HSTS hlavičku v `SecurityHeadersMiddleware` -- `Strict-Transport-Security` se posílá jen v produkčním režimu.

- `true` (produkce, default) — prohlížeč pošle cookie **jen přes HTTPS**, server posílá HSTS. Nad HTTP se cookie neodešle, refresh selže.
- `false` (lokální vývoj) — cookie se posílá i přes plain HTTP, HSTS se nevysílá. Nutné pro vývoj na `http://localhost` (Vite dev server + Go backend jsou oba HTTP).

V `.env.example` je `false` kvůli dev workflow. V produkci **vždy** `true` + nasazení za TLS terminátor.

Ostatní flagy cookie jsou hardcoded, protože nemá smysl je měnit: `HttpOnly=true` (nepřístupné z JS, obrana proti XSS), `SameSite=Strict` (nepošle se při cross-site requestu, obrana proti CSRF), `Path=/api/v1/auth` (posílá se jen na auth endpointy).

### Documan

`DOCUMAN_HTTP_PORT=3005` — port pro `documan` Docker service definovaný v `docker-compose.yml`. Slouží jen pro lokální preview dokumentace, nesouvisí s aplikační binárkou.
