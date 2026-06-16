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

    // Sentry frontend config — injektováno do index.html při serve, SPA čte
    // za běhu. Backend Sentry (APP_SENTRY_DSN/RELEASE) se čte v cmd/ mimo
    // tuto strukturu. Viz podsekce Sentry níže + guide.
    FrontendSentryDSN string
    SentryEnvironment string
    SentryDebug       bool

    TrustProxyHeaders bool
    RateLimitLogin    string
    RateLimitRefresh  string
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
| `APP_TRUST_PROXY_HEADERS` | `false` | Číst klientskou IP z proxy hlaviček (`CF-Connecting-IP` → `X-Real-IP`) — zapnout **jen** za důvěryhodnou proxy, viz [níže](#app_trust_proxy_headers--cloudflare-origin-lock) |
| `APP_RATE_LIMIT_LOGIN` | `10/min` | Per-IP limit na `/auth/login` (prázdné = vypnuto) |
| `APP_RATE_LIMIT_REFRESH` | `60/min` | Per-IP limit na `/auth/refresh` (prázdné = vypnuto) |
| `APP_LOG_FORMAT` | `json` | Formát logu — `json` (produkce) nebo `text` (čitelný lokálně); cokoli ≠ `text` → `json`. Čteno přes `StartupConfig` (**ne** v hlavním Config struct) |
| `APP_LOG_LEVEL` | `info` | Minimální log level — `debug`\|`info`\|`warn`\|`error` (neznámá hodnota → `info`). Čteno přes `StartupConfig` (**ne** v hlavním Config struct) |
| `APP_SENTRY_DSN` | -- | Backend Sentry DSN (prázdné = vypnuto). Čteno přes `StartupConfig` (**ne** v hlavním Config struct) |
| `APP_SENTRY_DSN_FRONTEND` | -- | Frontend Sentry DSN — server ho injektuje do `index.html` jako `<meta>` tag |
| `APP_SENTRY_ENVIRONMENT` | `development` | Sentry environment, sdílené BE i FE. Když je DSN nastavený a tahle prázdná, appka při startu **varuje** (eventy by jinak tiše spadly pod `development`) |
| `APP_SENTRY_RELEASE` | (git tag) | Override release verze pro Sentry (jinak z git tagu při buildu). Čteno přes `StartupConfig` (linker-injected verze má přednost) |
| `APP_SENTRY_DEBUG` | `false` | Záměrné error triggery pro smoke-test Sentry. **Nikdy v produkci** |

> Config struct má **15 polí** (výše). `.env` snippet nahoře je jen ukázkový výřez; úplný seznam proměnných je v této tabulce a v `.env.example`. Kompletní nastavení Sentry (BE + FE projekty, DSN, deploy) je v [Sentry guide](/guides/sentry); jak observability funguje uvnitř viz [Observability](/framework/infrastructure/observability).

- `APP_JWT_SECRET` je povinný a musí mít **min. 32 znaků** (HS256 floor). Validace ho odmítne při startu — provádí ji `NewJwtService` (security), ne `LoadConfig` (ta jen parsuje durations); chybějící/krátký secret tak shodí konstrukci aplikace přes Wire.
- Duration proměnné se parsují přes `time.ParseDuration` (jediné, co může `LoadConfig` selhat).
- Bool proměnné parsují řetězec `"true"` jako `true`, vše ostatní jako `false`.

### APP_COOKIE_SECURE

Řídí `Secure` flag na refresh cookie, který prohlížeč používá pro `/api/v1/auth/refresh`. Stejný flag zároveň gate-uje HSTS hlavičku v `SecurityHeadersMiddleware` -- `Strict-Transport-Security` se posílá jen v produkčním režimu.

- `true` (produkce, default) — prohlížeč pošle cookie **jen přes HTTPS**, server posílá HSTS. Nad HTTP se cookie neodešle, refresh selže.
- `false` (lokální vývoj) — cookie se posílá i přes plain HTTP, HSTS se nevysílá. Nutné pro vývoj na `http://localhost` (Vite dev server + Go backend jsou oba HTTP).

V `.env.example` je `false` kvůli dev workflow. V produkci **vždy** `true` + nasazení za TLS terminátor.

Ostatní flagy **refresh** cookie jsou hardcoded, protože nemá smysl je měnit: `HttpOnly=true` (nepřístupné z JS, obrana proti XSS), `SameSite=Strict` (nepošle se při cross-site requestu, obrana proti CSRF), `Path=/api/v1/auth` (posílá se jen na auth endpointy). `APP_COOKIE_SECURE` gate-uje `Secure` flag u refresh cookie i u čitelné session-hint cookie `gk_session` (ta je záměrně **ne-`HttpOnly`** a na `Path=/`, nese jen flag `1` — viz [Auth guide](/guides/auth)).

### APP_TRUST_PROXY_HEADERS & Cloudflare origin-lock

Řídí, jak `IPExtractor` (v `presentation/http/middleware/ratelimit.go`) zjistí klientskou IP. Ta jedna hodnota teče do **tří míst**: per-IP rate-limitu (`/auth/login`, `/auth/refresh`), audit logu (`audit_log.actor_ip`) **i** strukturovaných logů a Sentry (`ip` v access logu, `user.ip_address` na zachycené chybě).

Pořadí rozlišení:

- `false` (default) — IP je **vždy** `RemoteAddr` (skutečná IP TCP spojení). Správné pro přímé vystavení bez proxy. Případné `CF-Connecting-IP` / `X-Real-IP` se ignorují, takže je klient nemůže podvrhnout.
- `true` — zkusí se v pořadí `CF-Connecting-IP` → `X-Real-IP` → `RemoteAddr`. `CF-Connecting-IP` je první schválně: za Cloudflare je `RemoteAddr` (a `X-Real-IP`) jen **edge IP Cloudflare**, kdežto `CF-Connecting-IP` nese skutečného návštěvníka. `X-Real-IP` zůstává jako fallback pro přímou reverse proxy (Traefik/nginx).

> ⚠️ **Origin-lock je povinný, ne volitelný.** HTTP hlavičky jsou důvěryhodné jen tak, jak je důvěryhodná síťová cesta. Pokud je origin (srv s aplikací) dosažitelný **přímo** — ne výhradně přes proxy — může kdokoliv poslat request s vymyšleným `CF-Connecting-IP: 1.2.3.4` a podvrhnout tím IP pro rate-limit (obejití zámku účtu), audit (falešná stopa) i logy. `APP_TRUST_PROXY_HEADERS=true` zapínej **jen** tehdy, když je origin firewallem omezen na adresní rozsahy proxy:
>
> - **Za Cloudflare:** povol na portech 80/443 příchozí spojení **jen z [rozsahů Cloudflare](https://www.cloudflare.com/ips/)** (IPv4 + IPv6) — na úrovni cloud firewallu (Hetzner/AWS), host firewallu (`ufw`/`iptables`/`nftables`) nebo proxy. Vše ostatní zahoď. Tím je `CF-Connecting-IP` nepodvrhnutelný, protože každý request fyzicky prošel přes Cloudflare.
> - **Za vlastní reverse proxy** (Traefik/nginx na stejném hostu/síti): origin nevystavuj veřejně (bind na loopback/privátní síť), proxy nech přepisovat `X-Real-IP`.
>
> Bez origin-locku nech `APP_TRUST_PROXY_HEADERS=false` — radši ztratíš skutečnou IP (uvidíš edge/proxy IP), než abys důvěřoval podvrhnutelné hodnotě.

### Logování

`APP_LOG_FORMAT` a `APP_LOG_LEVEL` (stejně jako backend Sentry proměnné) čte `config.LoadStartup()` — volané z `cmd/` na úplném začátku startu, ještě před `LoadConfig` — protože logger a reporter se staví jako první. Proto **nejsou** v hlavním `Config` struct, ale v `StartupConfig`. Obojí jde přes stejný `getEnv` helper, takže `os.Getenv` žije v jediném místě (žádné raw čtení roztroušené po `cmd/`).

- **`APP_LOG_FORMAT`** — `json` (default, pro produkci a log agregaci) nebo `text` (čitelný handler pro lokální vývoj). Cokoli jiného než `text` spadne na `json`.
- **`APP_LOG_LEVEL`** — `debug` \| `info` (default) \| `warn` \| `error`. Neznámá nebo prázdná hodnota → `info`.

Veškeré logování jde **jedinou cestou** přes injektovaný `*slog.Logger` (staticky vynuceno lintem — `depguard`/`forbidigo`/`sloglint`). Logy úrovně `INFO+` se zároveň propisují do Sentry breadcrumbs. Detaily viz [Observability](/framework/infrastructure/observability).

### Sentry

Sentry config je rozdělené na dvě cesty, protože backend a frontend ho potřebují v jiný okamžik:

- **Backend** — `APP_SENTRY_DSN`, `APP_SENTRY_ENVIRONMENT`, `APP_SENTRY_RELEASE` čte `config.LoadStartup()` (volané z `cmd/main.go`) **ještě před** `LoadConfig`, protože reporter se staví spolu s loggerem na začátku startu. Proto **nejsou** v hlavním `Config` struct, ale v `StartupConfig`.
- **Frontend** — `APP_SENTRY_DSN_FRONTEND` + sdílené `APP_SENTRY_ENVIRONMENT` + `APP_SENTRY_DEBUG` jsou v `Config` struct, protože je server za běhu **injektuje do `index.html`** jako `<meta name="gokick:…">` tagy (jeden buildnutý image tak slouží všem prostředím). SPA je čte přes `runtimeConfig.ts`. Release verze je u FE výjimka — zůstává zapečená při buildu (`VITE_SENTRY_RELEASE`), protože image je per-verze.
- **`APP_SENTRY_DEBUG=true`** odemkne záměrné error triggery (BE `GET /debug/sentry` panika + FE tlačítko) pro ověření Sentry end-to-end na deploy. Appka při startu varuje; **nikdy nezapínej v produkci**.

Kompletní postup nastavení (založení projektů, DSN, CSP, deploy za Cloudflare) je v [Sentry guide](/guides/sentry). Jak error reporting funguje uvnitř (port `ErrorReporter`, obohacení eventu, whitelist) viz [Observability](/framework/infrastructure/observability).

### Documan

`DOCUMAN_HTTP_PORT` (default `3005` v `.env.example`) — port pro `documan` Docker service. `docker-compose.yml` ho interpoluje přes `${DOCUMAN_HTTP_PORT}`, nehardcoduje. Slouží jen pro lokální preview dokumentace, nesouvisí s aplikační binárkou.
