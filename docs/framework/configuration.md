---
layout: 'page'
uri: '/framework/configuration'
position: 100
slug: 'framework-configuration'
parent: 'framework'
navTitle: 'Configuration'
title: 'Configuration'
description: 'Kompletní reference .env proměnných — co se čte, kdy, jaké jsou defaulty a kde se hodnoty validují.'
---

# Configuration

Celá aplikace se konfiguruje z prostředí — `.env` plus skutečné env proměnné, žádné globální stavové proměnné a žádné přímé volání `os.Getenv` roztroušené po kódu. Tahle stránka je **kompletní reference**: co každá `APP_*` proměnná dělá, jaký má default a kde se její hodnota validuje.

> Jak přidat/změnit env proměnnou a kde ji navázat: skill `/gk-config`. Plný seznam s komentáři: `.env.example` (zkopíruj na `.env` — viz [Installation](/framework/installation)). Kde tahle vrstva sedí v celku: [Architecture](/framework/architecture).


## Jak se konfigurace čte

Hodnoty se čtou na **dvou místech** podle toho, kdy je start potřebuje:

- **`Config` struct** — hlavní konfigurace, načtená přes `LoadConfig()` (v `infrastructure/config/config.go`). Volá se z DI při stavbě aplikace.
- **`StartupConfig`** — úzký výřez čtený přes `LoadStartup()` na úplném začátku `cmd/main.go`, **ještě před** `LoadConfig`, protože logger a error reporter se staví jako první (aby šlo zalogovat i selhání uvnitř `LoadConfig`).

Obě cesty čtou env přes jediný helper `getEnv(key, fallback)` — `os.Getenv` je tak na **jednom místě**, žádné přímé čtení roztroušené po `cmd/`. `getEnv` vrací fallback, když je proměnná prázdná nebo nenastavená.


## Kde se hodnoty validují

`LoadConfig` sama **nevaliduje obsah** — jen parsuje. Jediné, na čem může selhat, jsou dvě duration proměnné (`time.ParseDuration` na `APP_JWT_ACCESS_EXPIRATION` / `APP_JWT_REFRESH_EXPIRATION`). Bool proměnné parsují řetězec `"true"` jako `true`, vše ostatní jako `false`.

Sémantickou validaci dělají konzumenti:

- **`APP_JWT_SECRET`** je povinný a musí mít **min. 32 znaků** (HS256 floor, RFC 7518 §3.2). Kontrolu provádí `NewJwtService` (v `infrastructure/security/jwt.go`), **ne** `LoadConfig`. Chybějící/krátký secret tak shodí stavbu aplikace přes Wire DI při startu.
- **`APP_LOG_FORMAT` / `APP_LOG_LEVEL` / `APP_SENTRY_ENVIRONMENT`** mají v kódu fallback prázdný řetězec; jejich „efektivní default" (`json` / `info` / `development`) aplikují až konzumenti (`newLogHandler`, `parseLogLevel`, Sentry SDK) — viz poznámky u tabulek níže.


## HTTP server

| Proměnná | Default | Co dělá |
|---|---|---|
| `APP_HTTP_PORT` | `3000` | Port, na kterém naslouchá HTTP server (`./bin/app serve`). |
| `APP_CORS_ORIGIN` | `http://localhost:5173` | Jediný povolený origin pro CORS (Vite dev server). V produkci nastav na doménu SPA. |

Čteno přes `Config` struct.


## Database / SQLite

| Proměnná | Default | Co dělá |
|---|---|---|
| `APP_DB_PATH` | `./data/app.db` | Cesta k souboru SQLite databáze. |
| `APP_DB_JOURNAL_MODE` | `WAL` | SQLite journal mode. `WAL` je default a správná volba pro normální běh. Přepni na `DELETE`, když stejnou DB čte přes Docker bind mount jiný proces (např. prohlížení `data/app.db` v IDE, zatímco kontejner zapisuje) — virtualizovaný FS Docker Desktopu nezaručuje koordinaci mmap/shm, kterou WAL vyžaduje. |

Čteno přes `Config` struct. Ladění connection poolu, DSN (`_txlock=immediate`, `busy_timeout`, `foreign_keys`) a transakce viz skill `/gk-repositories` (+ `/gk-migrations` pro schéma).


## JWT / auth

| Proměnná | Default | Co dělá |
|---|---|---|
| `APP_JWT_SECRET` | (žádný) | HS256 podpisový klíč pro access tokeny. **Povinný**, min. 32 znaků — validuje `NewJwtService`, ne `LoadConfig` (viz výše). |
| `APP_JWT_ACCESS_EXPIRATION` | `15m` | Životnost krátkého JWT access tokenu. Parsuje `time.ParseDuration` — nevalidní hodnota shodí start. |
| `APP_JWT_REFRESH_EXPIRATION` | `168h` | Životnost dlouhého opaque refresh tokenu (7 dní). Parsuje `time.ParseDuration`. |

Čteno přes `Config` struct. Celý dvou-tokenový flow (rotace refresh tokenů, detekce krádeže, durable logout) viz skill `/gk-auth`.


## CORS / cookies

| Proměnná | Default | Co dělá |
|---|---|---|
| `APP_COOKIE_SECURE` | `true` | `Secure` flag na auth cookies + podmiňuje HSTS hlavičku (viz podsekce níže). `.env.example` má `false` kvůli lokálnímu vývoji nad HTTP. |
| `APP_CORS_ORIGIN` | `http://localhost:5173` | Povolený origin (viz HTTP server výše). |

Čteno přes `Config` struct.

### APP_COOKIE_SECURE

Řídí `Secure` flag na refresh cookie, kterou prohlížeč posílá na `/api/v1/auth/refresh`. Stejný flag zároveň podmiňuje HSTS hlavičku v `SecurityHeadersMiddleware` — `Strict-Transport-Security` se posílá jen v produkčním režimu.

- `true` (produkce, default) — prohlížeč pošle cookie **jen přes HTTPS**, server posílá HSTS. Nad plain HTTP se cookie neodešle a refresh selže.
- `false` (lokální vývoj) — cookie jde i přes HTTP, HSTS se nevysílá. Nutné pro vývoj na `http://localhost` (Vite dev server i Go backend jsou oba HTTP).

V produkci **vždy** `true` + nasazení za TLS terminátor.

Ostatní flagy **refresh** cookie jsou hardcoded, protože je nemá smysl měnit: `HttpOnly=true` (nepřístupné z JS, obrana proti XSS), `SameSite=Strict` (nepošle se cross-site, obrana proti CSRF), `Path=/api/v1/auth` (jen na auth endpointy). `APP_COOKIE_SECURE` podmiňuje `Secure` flag u refresh cookie i u čitelné session-hint cookie `gk_session` (ta je záměrně **ne-`HttpOnly`**, na `Path=/`, nese jen flag `1` — viz skill `/gk-auth`). HTTP hardening (CSRF, bezpečnostní hlavičky) viz skill `/gk-hardening`.


## Rate limiting & proxy

| Proměnná | Default | Co dělá |
|---|---|---|
| `APP_RATE_LIMIT_LOGIN` | `10/min` | Per-IP token-bucket limit na `/auth/login`. Formát `N/sec`, `N/min`, `N/hour` nebo `N/Xs\|m\|h`. Prázdné = vypnuto. |
| `APP_RATE_LIMIT_REFRESH` | `60/min` | Per-IP limit na `/auth/refresh`, stejný formát. Prázdné = vypnuto. |
| `APP_TRUST_PROXY_HEADERS` | `false` | Číst klientskou IP z proxy hlaviček místo `RemoteAddr` (viz podsekce níže). Zapnout **jen** za důvěryhodnou proxy. |

Čteno přes `Config` struct. Detaily o lockoutu účtu a konstantním čase loginu viz skill `/gk-rate-limiting`.

### APP_TRUST_PROXY_HEADERS & Cloudflare origin-lock

Řídí, jak `IPExtractor` (v `presentation/http/middleware/ratelimit.go`) zjistí klientskou IP. Ta jedna hodnota teče do **tří míst**: per-IP rate-limitu (`/auth/login`, `/auth/refresh`), audit logu (`audit_log.actor_ip`) **i** strukturovaných logů a Sentry (`ip` v access logu, `user.ip_address` na zachycené chybě).

Pořadí rozlišení:

- `false` (default) — IP je **vždy** `RemoteAddr` (skutečná IP TCP spojení). Případné `CF-Connecting-IP` / `X-Real-IP` se ignorují, takže je klient nemůže podvrhnout.
- `true` — zkusí se v pořadí `CF-Connecting-IP` → `X-Real-IP` → `RemoteAddr`. `CF-Connecting-IP` je první schválně: za Cloudflare je `RemoteAddr` (a `X-Real-IP`) jen **edge IP Cloudflare**, kdežto `CF-Connecting-IP` nese skutečného návštěvníka. `X-Real-IP` zůstává fallback pro přímou reverse proxy (Traefik/nginx).

> ⚠️ **Origin-lock je povinný, ne volitelný.** HTTP hlavičky jsou důvěryhodné jen tak, jak je důvěryhodná síťová cesta. Pokud je origin (server s aplikací) dosažitelný **přímo** — ne výhradně přes proxy — může kdokoliv poslat request s vymyšleným `CF-Connecting-IP: 1.2.3.4` a podvrhnout tím IP pro rate-limit (obejití zámku účtu), audit (falešná stopa) i logy. `APP_TRUST_PROXY_HEADERS=true` zapínej **jen** tehdy, když je origin firewallem omezen na rozsahy proxy:
>
> - **Za Cloudflare:** povol na portech 80/443 příchozí spojení **jen z [rozsahů Cloudflare](https://www.cloudflare.com/ips/)** (IPv4 + IPv6) — na úrovni cloud firewallu (Hetzner/AWS), host firewallu (`ufw`/`iptables`/`nftables`) nebo proxy. Vše ostatní zahoď. Tím je `CF-Connecting-IP` nepodvrhnutelný, protože každý request fyzicky prošel přes Cloudflare.
> - **Za vlastní reverse proxy** (Traefik/nginx na stejném hostu/síti): origin nevystavuj veřejně (bind na loopback/privátní síť), proxy nech přepisovat `X-Real-IP`.
>
> Bez origin-locku nech `APP_TRUST_PROXY_HEADERS=false` — radši ztratíš skutečnou IP (uvidíš edge/proxy IP), než abys důvěřoval podvrhnutelné hodnotě.


## Logging (APP_LOG_*)

| Proměnná | Efektivní default | Co dělá |
|---|---|---|
| `APP_LOG_FORMAT` | `json` | Formát logu: `json` (pro agregátory jako Loki) nebo `text` (čitelný lokálně, hodí se pro `make serve`). Cokoli kromě `text` se bere jako `json` — aplikuje `newLogHandler`. |
| `APP_LOG_LEVEL` | `info` | Minimální log level: `debug` \| `info` \| `warn` \| `error`. Neznámá nebo prázdná hodnota → `info` — aplikuje `parseLogLevel`. |

Čteno přes **`StartupConfig`** (ne v hlavním `Config` struct), protože logger se staví jako úplně první věc v `main`. V kódu je `getEnv`-fallback prázdný řetězec; efektivní default výše aplikuje až konstrukce loggeru.

Veškeré logování jde **jedinou cestou** přes injektovaný `*slog.Logger` (staticky vynuceno lintem — `depguard`/`forbidigo`/`sloglint`). Když je zapnuté Sentry, logy úrovně `INFO+` se zároveň propisují do Sentry breadcrumbs (přes `breadcrumbHandler`). Detaily viz skill `/gk-logging`.


## Sentry (APP_SENTRY_*)

Konfigurace Sentry je rozdělená na **dvě cesty**, protože backend a frontend ji potřebují v jiném okamžiku a jako oddělené Sentry projekty (proto dva DSN). DSN je veřejný, takže ho lze bezpečně vystavit.

| Proměnná | Default | Čteno kde | Co dělá |
|---|---|---|---|
| `APP_SENTRY_DSN` | (prázdné = vypnuto) | `StartupConfig` | Backend Sentry DSN. Čte ho Go binárka za běhu, reporter se staví v `cmd/sentry.go`. Prázdné → `NopReporter`, aplikace běží beze změny. |
| `APP_SENTRY_DSN_FRONTEND` | (prázdné = vypnuto) | `Config` | Frontend Sentry DSN. Server ho za běhu injektuje do `index.html` jako `<meta name="gokick:sentry-dsn">`, SPA ho čte přes `runtimeConfig.ts`. |
| `APP_SENTRY_ENVIRONMENT` | (prázdné → SDK použije `development`) | `StartupConfig` + `Config` | Sentry environment, sdílené BE i FE. Když je DSN nastavený a tohle prázdné, `main.go` při startu **varuje** — eventy by jinak tiše spadly pod `development`. |
| `APP_SENTRY_RELEASE` | (git tag z buildu) | `StartupConfig` | Override release verze pro Sentry. Normálně se nenastavuje — verzi injektuje linker (`-X main.release`); env-hodnota se použije jen jako fallback, když binárka není stampnutá. |
| `APP_SENTRY_DEBUG` | `false` | `Config` | Odemkne **záměrné spouštěče chyb** pro end-to-end smoke-test Sentry: BE `GET /debug/sentry` vyvolá paniku, FE zobrazí tlačítko, které vyvolá chybu. Aplikace při startu varuje. **Nikdy v produkci.** |

> Pozor na default `APP_SENTRY_ENVIRONMENT` i `APP_SENTRY_DSN_FRONTEND`: v `.env.example` je `APP_SENTRY_ENVIRONMENT=development`, ale **programový** fallback v `getEnv` je prázdný řetězec — efektivní `development` přidá až Sentry SDK / meta-tag fallback na frontendu.

**Frontend release** je výjimka — zůstává zapečený při buildu (`VITE_SENTRY_RELEASE`), protože Docker image je per-verze, kdežto DSN/environment se injektují za běhu (jeden image slouží všem prostředím).

Kompletní postup (založení projektů, DSN, CSP, deploy za Cloudflare, source maps) je v skillu `/gk-sentry`. Co se reportuje a co ne (jen neočekávaná selhání — paniky, terminálně padlé joby, Vue chyby) viz skill `/gk-sentry`.


## Ostatní proměnné v .env.example

Tyto proměnné běžící HTTP server **nepoužívá** — patří CLI příkazu `seed`, Vite dev serveru, nebo build/CI:

| Proměnná | Default | Co dělá |
|---|---|---|
| `APP_SEED_ADMIN_PASSWORD` | (žádný) | Heslo admin uživatele pro `./bin/app seed` (8–128 znaků). Vyžaduje ho jen seed příkaz; v prostředích, kde se seed nikdy nespouští, nech prázdné. Wire ho injektuje jako distinct typ do seederu. |
| `VITE_SENTRY_DSN`, `VITE_SENTRY_ENVIRONMENT` | (prázdné) | Frontend Sentry config **jen pro Vite dev server** (`yarn dev`), kde se `index.html` doručuje přímo bez Go injekce. Když SPA obsluhuje Go server v produkci, tyto `VITE_*` se **nepoužijí** — přednost má injekce `APP_SENTRY_*`. |
| `VITE_SENTRY_RELEASE` | (git tag z buildu) | Frontend release, zapečený při buildu (viz Sentry výše). Normálně se nenastavuje. |
| `SENTRY_AUTH_TOKEN`, `SENTRY_ORG`, `SENTRY_PROJECT` | (žádné) | **Build-time** secrety pro upload frontend source maps. Patří do CI / Docker buildu, **ne** do runtime `.env`. Bez nich build neshipuje žádné mapy. Viz skill `/gk-sentry`. |
| `DOCUMAN_HTTP_PORT` | `3005` | Port pro `documan` Docker service (lokální preview dokumentace). `docker-compose.yml` ho interpoluje přes `${DOCUMAN_HTTP_PORT}`. Nesouvisí s aplikační binárkou. |

> `APP_SEED_ADMIN_PASSWORD` je technicky pole v `Config` struct (`SeedAdminPassword`), ale čte ho **jen** CLI příkaz `seed` — server běžící přes `serve` ho ignoruje, proto je uvedený zde.
