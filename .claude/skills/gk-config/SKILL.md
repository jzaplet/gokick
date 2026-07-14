---
layout: 'page'
uri: '/skills/gk-config'
position: 20
slug: 'skills-gk-config'
parent: 'skills-start'
navTitle: 'gk-config'
title: 'GK — Konfigurace z `.env`'
description: 'Konfigurace aplikace z .env — co se čte, kdy, jaké jsou defaulty a kde se hodnoty validují. Use when přidáváš/měníš env proměnnou, řešíš "odkud se bere tahle hodnota", nebo nechápeš, proč aplikace při startu selže kvůli konfiguraci.'
name: 'gk-config'
---

# GK — Konfigurace z `.env`

Veškeré nastavení aplikace teče z `.env` přes jeden reader. Žádné globální proměnné, `os.Getenv` žije na jediném místě.

## What & when

- Sáhni sem, když: přidáváš novou env proměnnou, ladíš „odkud se ta hodnota bere a jaký má default", nebo aplikace při startu selže kvůli konfiguraci.
- NEtýká se: jak se staví logger / error reporter z těch hodnot (to je observability), ani kompletní Sentry setup (deploy, DSN, CSP) — viz `Related`.

## For non-tech / juniors

`.env` je textový soubor s páry `KLÍČ=hodnota` (port, cesta k databázi, hesla, klíče). Aplikace ho při startu načte a podle něj se chová — proto se stejný zkompilovaný program dá rozjet lokálně i v produkci jen výměnou `.env`. Citlivé věci (klíče, hesla) tak nejsou zapečené v kódu.

Když proměnnou v `.env` nenastavíš, použije se vestavěný **default** (záložní hodnota). Kód má dvě „čtečky": jednu malou, co se spustí úplně první (aby fungovalo logování ještě než se načte zbytek), a jednu velkou pro vše ostatní.

## How it works

Dva config structy v `app/infrastructure/config/config.go`, oba čtou přes stejný `getEnv` helper:

- **`StartupConfig`** (`LoadStartup()`) — 5 polí: `LogFormat`, `LogLevel`, `SentryDSN`, `SentryEnvironment`, `SentryRelease`. Čte se **jako první** v `cmd/main.go`, **ještě před** `LoadConfig`. Důvod: logger a error reporter se staví hned na začátku startu, aby i selhání uvnitř `LoadConfig` šlo zalogovat a nahlásit.
- **`Config`** (`LoadConfig()`) — hlavní struct (15 polí: port, DB, JWT, CORS, cookie, rate-limit, frontend Sentry…). Načítá se přes Wire DI (`container_provider.go` → `wire_gen.go`), tedy později při stavbě aplikace.

`getEnv(key, fallback)` čte `os.Getenv` a **prázdný řetězec bere jako nenastaveno** → vrátí fallback. To je jediné místo s `os.Getenv` v celém repu (CLAUDE.md invariant: „read all env through the config reader, not raw os.Getenv").

```go
func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" { return v }
    return fallback
}
```

Pozn.: `APP_SENTRY_ENVIRONMENT` čtou **oba** structy (BE reporter ze `StartupConfig`, FE injekce do `index.html` z `Config`).

### Klíčové defaulty (kompletní tabulka v docs)

| Proměnná | Default v kódu | Pozn. |
|---|---|---|
| `APP_HTTP_PORT` | `3000` | |
| `APP_DB_PATH` | `./data/app.db` | |
| `APP_DB_JOURNAL_MODE` | `WAL` | `.env.example` má `DELETE` (bind-mount dev DB) |
| `APP_DB_MAX_CONNS` | `0` = auto | Pool cap. Auto = `clamp(2×NumCPU, 4, 32)`. SQLite serializuje zápisy → jde o paměť/backpressure, ne throughput |
| `APP_JWT_SECRET` | `""` | povinný, validuje `NewJwtService` (ne config) |
| `APP_JWT_ACCESS_EXPIRATION` | `15m` | parsuje `time.ParseDuration` |
| `APP_JWT_REFRESH_EXPIRATION` | `168h` | parsuje `time.ParseDuration` |
| `APP_CORS_ORIGIN` | `http://localhost:5173` | |
| `APP_COOKIE_SECURE` | `true` | `.env.example` má `false` (lokální HTTP) |
| `APP_RATE_LIMIT_LOGIN` | `10/min` | prázdné = vypnuto |
| `APP_RATE_LIMIT_REFRESH` | `60/min` | prázdné = vypnuto |
| `APP_TRUST_PROXY_HEADERS` | `false` | `true` jen za důvěryhodnou proxy |
| `APP_LOG_FORMAT` / `APP_LOG_LEVEL` | `""` → `json` / `info` | ve `StartupConfig` |
| `APP_SEED_ADMIN_PASSWORD` | `""` | povinný jen pro `./bin/app seed` |

Bool proměnné: parsuje se přesně řetězec `"true"` → `true`, cokoli jiného → `false` (`getEnv(...) == "true"`).

## Recipe

### Recipe: přidat novou env proměnnou

1. Rozhodni, **kdy** se čte:
   - logger/reporter na začátku startu → přidej pole do `StartupConfig` + řádek v `LoadStartup()`.
   - cokoli jiného (běh aplikace, DI) → přidej pole do `Config` + řádek v `LoadConfig()`.
2. Čti vždy přes `getEnv("APP_X", "<default>")` — nikdy `os.Getenv` napřímo.
3. Pro duration použij `time.ParseDuration` (vrať chybu jako u `APP_JWT_*`); pro bool `getEnv(...) == "true"`.
4. Přidej řádek do `.env.example` s komentářem (formát + default + kdy je potřeba).
5. Zdokumentuj v tabulce `docs/framework/configuration.md`.
6. Pokud měníš signaturu, kterou bere Wire → `make di`.

## Invariants & pitfalls

- **Jediná cesta k env je `getEnv`.** Žádné `os.Getenv` roztroušené po `cmd/` — vše přes config reader (CLAUDE.md invariant).
- **`LoadConfig` jen parsuje, nevaliduje doménově.** Jediné, na čem může selhat, je špatný duration (`APP_JWT_*`). Ostatní validace žijí jinde:
  - JWT secret min. **32 znaků** → `NewJwtService` (`minJWTSecretLen` v `app/infrastructure/security/jwt.go`). Chybějící/krátký secret shodí stavbu aplikace přes Wire, ne `LoadConfig`.
  - Journal mode whitelist `WAL|DELETE|MEMORY` → `NewSqliteManager` (`app/infrastructure/database/sqlite_manager.go`), ne config.
- **Default v kódu ≠ hodnota v `.env.example`** u dvou proměnných: `APP_COOKIE_SECURE` (kód `true`, soubor `false`) a `APP_DB_JOURNAL_MODE` (kód `WAL`, soubor `DELETE`). `.env.example` je laděný na lokální dev; produkce drží kódové defaulty.
- **Prázdná hodnota = nenastaveno.** `KLÍČ=` v `.env` použije default (nejde takhle „vynutit prázdno" tam, kde default není prázdný).
- **`APP_SENTRY_DEBUG=true` nikdy v produkci** — odemyká záměrné spouštěče chyb; aplikace při startu varuje.

## Related

- Skills: `/gk-feature` (přidání env proměnné jako součást featury + `make di`), `/gk-architecture` (kam infrastructure config patří ve vrstvách)
- Docs: [Config](/framework/configuration) (kompletní tabulka všech proměnných)
- Kód: `app/infrastructure/config/config.go` (`Config`, `StartupConfig`, `LoadConfig`, `LoadStartup`, `getEnv`), `cmd/main.go` (čtení `LoadStartup` před vším), `app/infrastructure/di/container_provider.go` (`LoadConfig` ve Wire), `.env.example`
