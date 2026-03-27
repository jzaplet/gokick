---
layout: 'page'
uri: '/framework/config'
position: 2
slug: 'framework-config'
parent: 'framework'
navTitle: 'Konfigurace'
title: 'Konfigurace'
description: 'Konfigurace aplikace přes .env soubory – porty, databáze, JWT, CORS.'
---

# Konfigurace

Balíček `env/` načítá konfiguraci z `.env` souboru přes `godotenv`.


## Proměnné

```env
APP_HTTP_PORT=3000
APP_DB_PATH=./data/app.db
APP_JWT_SECRET=min-32-chars-random-secret-key-here
APP_JWT_ACCESS_EXPIRATION=15m
APP_JWT_REFRESH_EXPIRATION=168h
APP_CORS_ORIGIN=http://localhost:5173
```

| Proměnná | Default | Popis |
|---|---|---|
| `APP_HTTP_PORT` | `3000` | Port HTTP serveru |
| `APP_DB_PATH` | `./data/app.db` | Cesta k SQLite databázi |
| `APP_JWT_SECRET` | – | Klíč pro podepisování JWT (min. 32 znaků) |
| `APP_JWT_ACCESS_EXPIRATION` | `15m` | Životnost access tokenu |
| `APP_JWT_REFRESH_EXPIRATION` | `168h` | Životnost refresh tokenu (7 dní) |
| `APP_CORS_ORIGIN` | `http://localhost:5173` | Povolený CORS origin (Vite dev server) |


## Config struct

```go
// env/config.go
type Config struct {
    HTTPPort             string
    DBPath               string
    JWTSecret            string
    JWTAccessExpiration  time.Duration
    JWTRefreshExpiration time.Duration
    CORSOrigin           string
}
```

Config se vytvoří jednou při startu a injektuje přes Wire do všech služeb, které ho potřebují.
