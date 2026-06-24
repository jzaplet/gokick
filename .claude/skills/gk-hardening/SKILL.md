---
layout: 'page'
uri: '/skills/gk-hardening'
position: 40
slug: 'skills-gk-hardening'
parent: 'skills-auth'
navTitle: 'gk-hardening'
title: 'GK — HTTP hardening (CSRF, security headers, secret masking)'
description: 'HTTP hardening serveru — CSRF ochrana (Go 1.25 stdlib), bezpečnostní hlavičky (CSP/HSTS/…) a maskování tajemství před odchodem do error trackeru. Use when přidáváš/měníš middleware chain, ladíš zablokovaný cross-origin požadavek, upravuješ CSP kvůli externímu skriptu, nebo si ověřuješ, že se credential nedostane do Sentry.'
name: 'gk-hardening'
---

# GK — HTTP hardening (CSRF, security headers, secret masking)

Tři obranné vrstvy hned na vstupu HTTP serveru: odmítnutí cizích POST/PUT/DELETE (CSRF), bezpečnostní response hlavičky pro prohlížeč, a maskování tajemství těsně před odchodem do error trackeru.

## What & when

- Sáhni sem, když řešíš **middleware chain** (`buildMiddlewareChain`), **zablokovaný cross-origin** požadavek, **úpravu CSP** (přidáváš CDN / externí skript / embed), nebo si ověřuješ, že **credential neunikne do Sentry**.
- NEtýká se autentizace (JWT, hesla, permissions) — to je `security/` balíček, viz `Related`. Tohle je čistě **vstupní HTTP vrstva** + **výstupní bod (egress)** do error trackeru.

## For non-tech / juniors

Tři nezávislé pojistky, každá řeší jiný útok:

- **CSRF** (Cross-Site Request Forgery) = útok, kdy tě cizí web přiměje poslat akci na náš server pod tvojí přihlášenou identitou. Pojistka: server **odmítne stav měnící požadavek (POST/PUT/DELETE), který přišel z cizího webu**. Čtení (GET) projde.
- **Security headers** = pokyny pro prohlížeč „co smíš a co ne" (odkud načítat skripty, nezobrazovat nás v cizím iframu, mluvit jen po HTTPS…). Brání útokům jako XSS a clickjacking.
- **Secret masking** = než pošleme hlášení o chybě do Sentry, **přepíšeme tajemství** (hesla, tokeny, cookies) na `==MASKED==`, aby neutekla ven. Operátor pořád vidí, že hlavička dorazila — jen ne její obsah.

## How it works

Vše se skládá v `app/presentation/http/server/server.go` → `buildMiddlewareChain`. Pořadí (z komentáře v kódu):
`Trace → IP → ReportScope → Recovery → Security headers → CORS → CSRF → Logging → handler`.

### CSRF — Go 1.25 stdlib `http.CrossOriginProtection`

```go
csrf := &http.CrossOriginProtection{}   // prázdný = bezpečné defaulty
// ... v chainu: csrf.Handler
```

- Žádný **CSRF token, žádná cookie, žádný per-form secret**. `CrossOriginProtection` (stdlib Go 1.25) odmítne **stav měnící (non-safe-method) cross-origin** požadavek tak, že čte hlavičku `Sec-Fetch-Site`, a když chybí, porovná `Origin` vs `Host`. Safe metody (GET/HEAD) projdou vždy.
- **Prázdný struct** = žádné trusted cross-origins, žádné bypass patterns. To sedí na náš **same-origin SPA**: frontend obsluhuje stejný mux (`spa.Serve` catch-all v `registerRoutes`), takže API i stránka běží ze stejného originu a žádná výjimka není potřeba.
- CORS (`cors.go`) je vedlejší a oddělená věc — nastavuje `Access-Control-*` hlavičky pro povolený `APP_CORS_ORIGIN`; není to CSRF ochrana.

### Security headers — `SecurityHeadersMiddleware`

`app/presentation/http/middleware/security.go` → `SecurityHeadersMiddleware(hstsEnabled bool, sentryDSN string)`. Posílá na každou odpověď:

- **`Content-Security-Policy`** (CSP) — `default-src 'self'`, `script-src 'self'`, `style-src 'self' 'unsafe-inline'` (Vue injektuje scoped styly inline), `img-src/font-src 'self' data:`, `object-src 'none'`, `frame-ancestors 'none'` (= nelze nás vložit do iframu). Když je nastavený **frontend Sentry DSN**, jeho ingest origin se přidá do `connect-src` (jinak by CSP zablokovala odeslání chyby) — viz `sentryIngestOrigin`.
- **`X-Content-Type-Options: nosniff`**, **`X-Frame-Options: DENY`**, **`Referrer-Policy: strict-origin-when-cross-origin`**, **`Permissions-Policy`** (široký deny: camera, microphone, geolocation, payment, …), **`Cross-Origin-Opener-Policy`** + **`Cross-Origin-Resource-Policy: same-origin`**.
- **`Strict-Transport-Security`** (HSTS) — pošle se **jen když `hstsEnabled == true`**. V `server.go` je to napojené na `s.config.CookieSecure` (env **`APP_COOKIE_SECURE`**), které už rozlišuje HTTPS provoz. **Není žádný samostatný `APP_HSTS` flag.**

### Secret masking — `domain/shared/mask.go`

Maskuje se **na jediném místě: těsně před odchodem do error trackeru** (`ErrorReporter` / Sentry). **Normální stdout slog výstup se nemaskuje** — komentáře v kódu to říkají doslova („bound for the error tracker"). Dvě různé strategie podle seamu:

- **Request hlavičky → allow-list** (`sensitiveHeaderNames`: `authorization`, `cookie`, `set-cookie`, `x-api-key`, …). `MaskHeaderValue` u `Authorization`/`Proxy-Authorization` **nechá schéma** (`Bearer ==MASKED==`), zbytek (Cookie…) přepíše celý. Volá se v `recovery.go` před `Capture` a v `cmd/sentry.go` (`sentryRequest`, `maskRequestHeaders`).
- **Breadcrumbs / tags → key-substring heuristika** (`IsSensitiveLogKey`: klíč obsahuje `password`, `token`, `secret`, `api_key`, …). Tento seam (slog→breadcrumb most) **nemá whitelist**, takže se raději **přemaskuje** než aby něco uniklo — `cmd/sentry.go` `scrubBreadcrumb` + `SetTag`.

`mask.go` jsou **čisté funkce bez závislostí** v `domain/shared/`, právě proto na ně smí sáhnout jak `presentation` (recovery.go), tak `cmd/` (sentry.go), aniž by porušily pravidla vrstev.

## Recipe: povolit externí skript / origin v CSP

1. Otevři `app/presentation/http/middleware/security.go`, najdi `csp := strings.Join([]string{ … })`.
2. Přidej host do správné direktivy (`script-src` pro skript, `connect-src` pro XHR/fetch, `img-src` pro obrázky…). Drž to **co nejužší** — CSP je záměrně lokální (`'self'`).
3. Sentry ingest se přidává automaticky přes `sentryIngestOrigin` — ten needituj ručně.
4. `make test` (security_test pokrývá direktivy) + ověř v prohlížeči (DevTools → Console hlásí CSP violations).

## Recipe: nová credential hlavička / log klíč k maskování

1. **Hlavička** (její hodnota je tajemství) → přidej název do `sensitiveHeaderNames` v `mask.go`.
2. **Log/tag/breadcrumb klíč** → přidej podřetězec do `sensitiveKeySubstrings`. Match je case-insensitive substring, takže `*_token`, `*password*` chytíš jedním záznamem.
3. `make test` — `mask_test.go` ověří zachování schématu.

## Invariants & pitfalls

- **CSRF není token-based.** Nehledej CSRF cookie ani hidden field — ochrana je čistě origin-check (`Sec-Fetch-Site` / `Origin` vs `Host`). Cross-origin POST z cizího webu je odmítnut, same-origin SPA projde.
- **HSTS se řídí podle `APP_COOKIE_SECURE`, ne podle vlastního flagu.** Lokálně přes HTTP se HSTS neposílá (a to je správně).
- **Maskuje se jen egress do error trackeru.** Nepředpokládej, že běžný log na stdout je maskovaný; jediné dvě cesty jsou recovery `Capture` a Sentry adapter v `cmd/sentry.go`.
- **Pořadí v chainu drž.** Trace/IP/ReportScope jen plní `ctx` a běží PŘED Recovery (aby každý panic report nesl trace_id + IP + breadcrumbs); Security headers běží před handlerem. Když přidáváš middleware, nedávej ho mezi tyto ctx-plnící middleware a Recovery bez důvodu.
- **CSP `'unsafe-inline'` je jen u `style-src`** kvůli Vue scoped stylům — nikdy ho nepřidávej do `script-src`.
- **`mask.go` nesmí získat závislosti.** Patří do `domain/shared/`, importuje jen stdlib (`strings`) — drž ho čistý, jinak rozbiješ import obou konzumentů (presentation + cmd).

## Related

- `/gk-config` — `APP_COOKIE_SECURE` (gate pro HSTS) a `APP_SENTRY_DSN_FRONTEND` (origin do CSP `connect-src`), odkud se čtou a defaulty.
- `/gk-architecture` — sestavení middleware chainu, vrstvy a proč `mask.go` žije v `domain/shared/`.
- `/gk-rate-limiting` — další obrana auth endpointů na vstupu (per-IP 429, lockout, konstantní čas loginu) — sousedí v chainu s CSRF/security headers.
- `/gk-auth` — autentizace (JWT, refresh, theft detection, cookies); tahle skill je vstupní HTTP vrstva, ne login flow.
- `/gk-logging` — jediná slog cesta a `LogKey*` konstanty; maskování chrání právě egress té log stream do trackeru.
- `/gk-errors` — jak recovery a handlery mapují chyby na HTTP status (maskování řeší jen co jde do trackeru, ne odpověď klientovi).
- Kód: `app/presentation/http/middleware/security.go` (`SecurityHeadersMiddleware`, `sentryIngestOrigin`), `app/domain/shared/mask.go`, `app/presentation/http/server/server.go` (`buildMiddlewareChain` — CSRF wiring), `app/presentation/http/middleware/recovery.go` (mask před `Capture`), `cmd/sentry.go` (`sentryRequest`/`maskRequestHeaders`/`scrubBreadcrumb`).
