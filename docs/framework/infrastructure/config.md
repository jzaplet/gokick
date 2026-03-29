---
layout: 'page'
uri: '/framework/infrastructure/config'
position: 1
slug: 'framework-infrastructure-config'
parent: 'framework-infrastructure'
navTitle: 'Konfigurace'
title: 'Konfigurace'
description: 'Balíček env/ – .env soubory, Config struct.'
---

# Konfigurace

Balíček `env/`. Načítá konfiguraci z `.env` přes `godotenv`.


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
| `APP_DB_PATH` | `./data/app.db` | Cesta k SQLite |
| `APP_JWT_SECRET` | – | JWT klíč (min. 32 znaků) |
| `APP_JWT_ACCESS_EXPIRATION` | `15m` | Access token životnost |
| `APP_JWT_REFRESH_EXPIRATION` | `168h` | Refresh token životnost |
| `APP_CORS_ORIGIN` | `http://localhost:5173` | CORS origin |


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
