---
name: gk-rate-limiting
description: Obrana auth endpointů proti hádání hesel a zahlcení — per-IP token bucket (429), lockout účtu po 5 selháních, konstantní čas loginu (neprozradí, jestli login existuje) a jednotná extrakce klientské IP. Use when ladíš/měníš rate limit na /login či /refresh, řešíš lockout účtu, 429 v testech, nebo „odkud aplikace bere klientskou IP" za reverse proxy.
layout: 'page'
uri: '/skills/gk-rate-limiting'
position: 30
slug: 'skills-gk-rate-limiting'
parent: 'skills-auth'
navTitle: 'gk-rate-limiting'
title: 'GK — Rate limiting & brute-force ochrana'
---

# GK — Rate limiting & brute-force ochrana

Tři vrstvy obrany auth endpointů: per-IP token bucket (počet requestů za čas), zámek účtu po opakovaných selháních a login, který odpovídá za konstantní čas.

## What & when

- Sáhni sem, když: měníš/ladíš limity na `POST /api/v1/auth/login` nebo `/auth/refresh`, dostáváš `429 Too Many Requests`, řešíš zamčený účet (login vrací „invalid credentials" i se správným heslem), nebo nevíš, odkud se bere klientská IP za proxy/Cloudflarem.
- NEtýká se: vlastní autentizace (tokeny, session lifecycle, theft detection) ani permission kontroly. Token bucket a lockout jen **chrání vstup**; co se děje po úspěšném loginu, řeší `/gk-auth` (auth flow) a `/gk-permissions`.

## For non-tech / juniors

Útočník zkouší hesla naslepo (brute-force) nebo aplikaci zahltí spoustou requestů. Bráníme se třemi nezávislými způsoby:

1. **Token bucket (per-IP).** Představ si kbelík s žetony pro každou IP adresu. Každý request jeden žeton vyndá, kbelík se plynule doplňuje (např. 10 žetonů za minutu). Když je prázdný, request dostane `429` a hlavičku `Retry-After` (za kolik vteřin to zkusit znovu). Pomáhá proti záplavě z jedné IP.
2. **Lockout účtu.** Token bucket nepomůže, když útočník střídá IP adresy. Proto navíc: po **5 špatných pokusech během 10 minut** se účet zamkne na **15 minut** — bez ohledu na to, z kolika IP útok přišel.
3. **Konstantní čas loginu.** Login schválně trvá stejně dlouho, ať uživatel existuje, neexistuje, nebo je zamčený. Kdyby „neexistující jméno" odpovídalo rychleji, útočník by si podle času odvodil, která jména jsou platná. Všechny chyby vrací stejnou neutrální hlášku `invalid credentials`.

## How it works

### Per-IP token bucket (`app/presentation/http/middleware/ratelimit.go`)

- `RateRule{Tokens, Per}` = „N requestů za dobu". Parsuje se z configu přes `ParseRateRule` (formáty `N/sec`, `N/min`, `N/hour`, `N/Xs|Xm|Xh`). **Prázdný string → nulové `RateRule` = vypnuto** (middleware je pass-through).
- `RateLimiter` drží `map[string]*bucket` (klíč = IP). `allow()` je pod jedním mutexem: doplní žetony podle uplynulého času, ořízne na strop, a když je `< 1`, vrátí `false`.
- Při vyčerpání middleware nastaví `Retry-After` (= `int(Per.Seconds())`) a vrátí `429` přímo přes `response.Error` — **neprochází** doménovým error→HTTP mapováním (viz `/gk-errors`).
- **Janitor:** `Run(ctx, interval, dropAfter)` v goroutině maže buckety idle déle než `dropAfter`, aby mapa nerostla pod stuffing útokem. Server je pouští s `janitorSweepInterval = 1 min`, `janitorDropAfter = 5 min` (`server.go`).

Buckety jsou **per-IP a per-endpoint** — login a refresh mají oddělené limitery (`server.RateLimiters{Login, Refresh}`). Aplikují se jen na ty dvě cesty (`server.go`):

```go
mux.Handle("POST /api/v1/auth/login",   loginLimit(http.HandlerFunc(s.auth.Login)))
mux.Handle("POST /api/v1/auth/refresh", refreshLimit(http.HandlerFunc(s.auth.Refresh)))
```

Defaulty (`config.go`): `APP_RATE_LIMIT_LOGIN = 10/min`, `APP_RATE_LIMIT_REFRESH = 60/min`.

### Extrakce klientské IP (`app/presentation/http/middleware/ip.go`)

`NewIPExtractor(trustProxy)` je **jediný zdroj pravdy** pro klientskou IP — sdílí ho rate limiter i audit, takže se jejich IP nikdy nerozejdou.

- `trustProxy == false` (default): IP = `RemoteAddr` (host bez portu). Bezpečné, klient ji nemůže podvrhnout.
- `trustProxy == true`: pořadí `CF-Connecting-IP` → `X-Real-IP` → `RemoteAddr`. (Cloudflare jinak schová návštěvníka za svou edge IP.)
- `IPMiddleware` zavolá extractor jednou a uloží IP do contextu (`shared.ContextWithActorIP`); audit ji pak razítkuje na každý `AuditRecord`.

Řídí to `APP_TRUST_PROXY_HEADERS` (viz `/gk-config`).

### Lockout + konstantní čas (`app/application/auth/command/login.go`)

`LoginHandler.Handle` drží jednotné časování i lockout (konstanty v souboru: `loginLockThreshold = 5`, `loginLockWindow = 10m`, `loginLockDuration = 15m` — schválně hardcoded, ne env):

1. Najde uživatele podle nicku. **Vždy** zavolá `password.Verify` — i když uživatel neexistuje (proti `dummyHash` napočítanému při startu) nebo je zamčený. Tím každá větev zaplatí stejný bcrypt čas → response neprozradí, jestli jméno existuje ani jestli je účet zamčený.
2. Lock check (`u.LockedUntil`) je **až po** `Verify`, aby zamčená větev nebyla měřitelně rychlejší.
3. Selhání (neznámý / špatné heslo) → `handleFailedLogin`: audit `auth.login.failed`, a pro známé a nezamčené uživatele `users.RecordFailedLogin(...)`; když pokus překročí threshold, audit `auth.account.locked`.
4. Zamčený účet se správným heslem → audit `auth.login.blocked_while_locked`, vrátí stejný neutrální `AuthError` (401).
5. Úspěch → `ResetFailedLogin` (počítadlo na 0), vydá tokeny, audit `auth.login.succeeded`.

**Atomicita počítadla** (`infrastructure/sqlite/user/repository.go`): `RecordFailedLogin` je jediný SQL `UPDATE ... CASE` — rozhodne reset (mimo okno) / inkrement / lock (na/přes threshold) atomicky, žádný read-modify-write race. `RecordFailedLogin` i `ResetFailedLogin` zapisují přes **raw pool** (`r.DB.DB()`), ne přes tx-aware `r.Conn(ctx)` — jsou to single-statement auto-commity, takže počítadlo přežije nezávisle na jakékoli okolní transakci. Navíc `LoginCommand` deklaruje `SkipTransaction()`, aby handler nebyl obalený write-tx, která by se s tím raw-pool zápisem pod SQLite zadeadlockovala (viz `/gk-repositories`).

## Recipe

### Recipe: změnit rate limit endpointu
1. Nastav `APP_RATE_LIMIT_LOGIN` / `APP_RATE_LIMIT_REFRESH` v `.env` (formát `N/min`, `N/sec`, `N/hour`, `N/30s`…). **Prázdná hodnota = limit vypnutý.**
2. Restartuj server — rule se parsuje při startu v `provideRateLimiters`; **neplatný formát = error a server nenastartuje** (fail-fast).

### Recipe: nasazení za reverse proxy / Cloudflare
1. Zapni `APP_TRUST_PROXY_HEADERS=true` — **jen** pokud je origin firewallnutý tak, že jde dosáhnout výhradně přes proxy.
2. Ověř, že proxy posílá `CF-Connecting-IP` (Cloudflare) nebo `X-Real-IP` (Traefik/nginx). Bez toho by rate limit i audit braly IP proxy, ne návštěvníka.

### Recipe: ladění „proč je účet zamčený"
1. Zkontroluj řádek v `users`: `failed_login_attempts`, `last_failed_login_at`, `locked_until` (viz `/gk-entities`).
2. V `audit_log` najdeš `auth.login.failed`, `auth.account.locked`, `auth.login.blocked_while_locked`.
3. Odemčení: počkej na vypršení `locked_until`, nebo úspěšný login (`ResetFailedLogin` vynuluje počítadlo i lock).

## Invariants & pitfalls

- **`APP_TRUST_PROXY_HEADERS` zapínej jen za důvěryhodnou cestou.** Když je origin dosažitelný přímo (ne jen přes proxy), libovolný klient si `CF-Connecting-IP` / `X-Real-IP` podvrhne a obejde rate limit i zfalšuje audit IP. Default `false` je bezpečný.
- **Login musí pořád volat `Verify`.** Neoptimalizuj „uživatel neexistuje" na rychlý return ani lock check před `Verify` — rozbiješ konstantní čas a začneš leakovat existenci/lock přes timing.
- **Všechny login chyby = jeden neutrální `AuthError`.** Neznámé jméno, špatné heslo i lock vrací stejné `invalid credentials` (401). Nikdy nerozlišuj hlášku podle příčiny.
- **Lock konstanty jsou v kódu, ne v env** (`login.go`) — záměrně, aby ochranu nešlo nechtěně oslabit přes config. Měň je tam, ne přidáváním env proměnné.
- **Counter zápisy přes raw pool jsou vědomá výjimka** z pravidla „v repozitáři vždy `r.Conn(ctx)`". Nepřepisuj je na tx-aware connection — počítadlo brute-force by se rozbilo (viz `/gk-repositories`).
- **429 z limiteru neprochází doménovým error mapováním.** Nastaví ho přímo middleware s `Retry-After`; doménový `AuthError` → 401 je oddělená cesta (viz `/gk-errors`).
- **Limiter je in-process, per-replika.** Dvě repliky mají oddělené buckety — efektivní per-IP limit je `N × počet replik`. Lockout je naopak v DB, tedy globální.

## Related

- Skills: `/gk-auth` (login/refresh/logout flow, rotace tokenů, theft detection — to, co rate limiting chrání), `/gk-config` (env `APP_RATE_LIMIT_*`, `APP_TRUST_PROXY_HEADERS`), `/gk-bus` (middleware chain, `SkipTransaction`, audit mimo tx), `/gk-errors` (401/429 mapování), `/gk-repositories` (raw pool výjimka, SQLITE_BUSY), `/gk-entities` (lock sloupce na `users`), `/gk-permissions` (autorizace po loginu)
- Kód: `app/presentation/http/middleware/ratelimit.go`, `app/presentation/http/middleware/ip.go`, `app/presentation/http/server/server.go`, `app/application/auth/command/login.go`, `app/infrastructure/sqlite/user/repository.go`, `app/infrastructure/di/container_provider.go`
