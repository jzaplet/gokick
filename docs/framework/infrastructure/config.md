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

- `LoadConfig()` vrací error pokud `APP_JWT_SECRET` chybí -- je povinný.
- Duration proměnné se parsují přes `time.ParseDuration`.
