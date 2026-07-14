---
layout: 'page'
uri: '/skills/gk-auth'
position: 10
slug: 'skills-gk-auth'
parent: 'skills-auth'
navTitle: 'gk-auth'
title: 'GK — Autentizace (JWT access + opaque refresh)'
description: 'Autentizace přes dva tokeny (krátký JWT access + dlouhý opaque refresh), rotace refresh tokenů s detekcí krádeže, login odolný proti timing útoku a durable logout. Use when řešíš login/refresh/logout flow, session cookies, "proč mě to odhlásilo", theft detection nebo brute-force lock účtu.'
name: 'gk-auth'
---

# GK — Autentizace (JWT access + opaque refresh)

Dvoutokenový systém: krátkožijící podepsaný **access token** pro každý request a
dlouhožijící náhodný **refresh token** v cookie, který se při každém použití rotuje
a hlídá proti krádeži.

## What & when
- Sáhni sem, když: implementuješ/ladíš `login` / `refresh` / `logout`; řešíš refresh
  a session cookies; nechápeš, proč request padá na `401`; ladíš theft detection
  (token reuse) nebo brute-force lock účtu; řešíš, proč se uživatel "sám odhlásil".
- NEtýká se: kdo smí volat který endpoint a jak se skládají permission stringy →
  `/gk-permissions`. Kde se `AuthError` mapuje na HTTP `401` → `/gk-errors`. Pořadí
  middleware (Authorize, Transaction) a `SkipTransaction` mechanika → `/gk-bus`.
  Env hodnoty (`APP_JWT_SECRET`, expirace, `APP_COOKIE_SECURE`) → `/gk-config`.

## For non-tech / juniors
Představ si **denní vstupenku** a **členskou kartu**. Vstupenka (access token) platí
pár minut a ukazuješ ji u každých dveří — je rychlá na ověření, ale brzy propadne.
Členská karta (refresh token) platí dlouho, drží se bezpečně schovaná (v cookie, kterou
JavaScript nepřečte) a slouží jen k tomu, aby ti u přepážky vytiskli novou vstupenku.

Klíčový trik: při každém výtisku ti **starou kartu znehodnotí a vydají novou**. Když se
ta samá stará karta objeví podruhé, systém pozná, že ji někdo zkopíroval (ukradl), a
**okamžitě zruší všechny karty** daného uživatele — útočník i ty jste odhlášeni, ať si
heslo nastaví nanovo.

## How it works

**Dva tokeny** (`app/infrastructure/security/jwt.go`):
- **Access token** — HS256-podepsaný JWT, claims `sub` (UserID), `role`, `nickname`,
  `email`, `tenant` (TenantID pro multitenancy), `iat`, `exp`. Krátká expirace
  (`APP_JWT_ACCESS_EXPIRATION`, default `15m`).
  Posílá se v `Authorization: Bearer`, na FE žije jen v paměti (`setAccessToken`).
- **Refresh token** — náhodný řetězec z `rand.Text()` (Go 1.24+). Klientovi jde **raw**
  hodnota v cookie; v DB se ukládá jen jeho **SHA-256 hash** (`HashToken`). Server proto
  raw token nikdy nezná zpětně — příchozí token z cookie zhashuje a hledá podle hashe.

**Cookies** (`app/presentation/http/handler/auth.go`, `writeAuthResponse`):
- `refresh_token` — `HttpOnly` + `Secure` + `SameSite=Strict`, `Path=/api/v1/auth`
  (vidí ho jen auth endpointy). Expirace = expirace refresh tokenu.
- `gk_session=1` — **čitelná** (ne-`HttpOnly`) "session hint" cookie, `Path=/`, **stejná
  expirace**. SPA `HttpOnly` refresh cookie z JS nevidí, takže by host (nepřihlášený) na
  každém načtení vystřelil zbytečný `POST /auth/refresh` → `401` v konzoli. Podle hintu
  se FE (`assets/app.ts:bootstrap` → `hasSessionHint()` z `assets/app-ui/Auth/sessionHint.ts`)
  rozhodne, jestli refresh při startu vůbec zkoušet.

**Rotace + theft detection** (`app/application/auth/command/refresh_token.go`): starý token
se při refreshi nemaže, jen označí `used_at = now` (atomicky přes `MarkUsed`, CAS z NULL).
Nový token se **uloží (`Save`) DŘÍV**, než se starý označí jako použitý. Dvě cesty ke krádeži:

| Situace | Akce |
|---|---|
| Token v DB není (`FindByHash` → nil) | `AuthError` (401) |
| Token už použitý (`UsedAt != nil`) | audit `auth.token.theft_detected` (`reused_after_rotation`) → `DeleteByUserID` → `AuthError` |
| `MarkUsed` netrefil řádek (CAS prohrál se souběhem) | audit `theft_detected` (`concurrent_rotation_race`) → `DeleteByUserID` → `AuthError` |
| Token expirovaný | `DeleteByUserID` (best-effort) → `AuthError` |
| Token platný | `Save` nový → `MarkUsed` starý → vydat novou dvojici |

`DeleteByUserID` smaže **všechny** tokeny usera = force logout na všech zařízeních.

**Login odolný proti timingu** (`app/application/auth/command/login.go`): `password.Verify`
se volá **vždy** — i pro neexistující nick (proti `dummyHash` napočítanému jednou v
`NewLoginHandler`). Lock-check běží **až po** `Verify`, takže "locked", "špatné heslo" i
"neznámý user" trvají stejně dlouho. Všechny tři padají na jeden neutrální
`AuthError{"invalid credentials"}` — i správné heslo na zamčeném účtu. Brute-force: po
`5` selháních v `10min` okně se účet zamkne na `15min` (konstanty v `login.go`); počítadlo
žije na `users` řádku, aktualizuje se přes raw pool **mimo** business tx (viz invarianty).

**Logout** (`app/application/auth/command/logout.go`): `DeleteByUserID(claims.UserID)`.
Jediný auth endpoint, který **vyžaduje permission** (`auth:logout`, tedy Bearer) — login
i refresh jsou `SkipPermissionCheck()` (veřejné).

**Endpointy:** `POST /api/v1/auth/login` (veřejný) · `POST /api/v1/auth/refresh`
(cookie) · `POST /api/v1/auth/logout` (Bearer). Login/refresh response:
`{ access_token, access_expiration, user: { id, nickname, email, role, permissions } }`.

## Recipe

### Recipe: pochopit/odladit jeden login→refresh→logout cyklus
1. **Login** — `LoginHandler.Handle` ověří heslo, resetuje fail-counter, vydá access JWT +
   uloží hash nového refresh tokenu, audit `auth.login.succeeded`. Handler nastaví obě
   cookies (`refresh_token` + `gk_session`).
2. **Refresh** — FE pošle cookie; handler zhashuje, najde token, zkontroluje
   reuse/expiry, `Save` nový → `MarkUsed` starý, vydá novou dvojici. FE
   (`assets/app-ui/Auth/state.ts:scheduleRefresh`) si plánuje auto-refresh ~30s před expirací.
3. **Logout** — `DeleteByUserID`, smažou se obě cookies. FE (`assets/app-ui/Auth/logout.ts`)
   ve `finally` zahodí in-memory stav + `clearSessionHint()`.

### Recipe: přidat nový claim do access tokenu
1. Přidej pole do `shared.AuthClaims` (`app/domain/shared/`).
2. Zapiš ho v `GenerateAccessToken` (`jwt.go`, do `jwt.MapClaims`) a přečti zpět ve
   `ValidateAccessToken` (přes `claimString`).
3. Naplň ho při vydání v `issueSession` (`issue_session.go`) — jediné místo, které staví
   `&shared.AuthClaims{...}` pro login i refresh (takhle tam přibyl claim `tenant`).
4. Pokud má jít na FE, přidej ho do structu `userDTO` (`auth.go`, plní se ve `writeAuthResponse`) a spusť `make ts-gen` — `userDTO` je `//gkts`-anotovaný, TS typ `AuthUser` se generuje a `make lint` (ts-check) bez regenerace spadne.

## Invariants & pitfalls
- **Do DB jen hash, nikdy raw refresh token.** Klient drží raw, server ho jen hashuje
  (`HashToken`) a porovnává. Únik DB tak refresh tokeny nevyzradí.
- **Pořadí `Save` nový → `MarkUsed` starý je závazné.** Obrácené pořadí by při selhání
  `Save` označilo starý token jako použitý a na příští pokus legitimního klienta
  vyhodnotilo jako theft → false logout. V tomto pořadí selhání `Save` nechá jen
  nepoužitý orphan token, ne odhlášení.
- **Cookie se maže IFF revokace fakt proběhla.** `logout` čistí cookie jen po úspěšném
  `DeleteByUserID`; `refresh` volá `clearRefreshCookie` jen na `*shared.AuthError` (401),
  **nikdy na 5xx**. Transientní výpadek (DB blip) tak neodhlásí platnou session — jinak by
  momentální chyba durably smazala session hint a příští bootstrap by refresh přeskočil.
- **Session hint expiruje stejně jako refresh cookie** a maže se jen na definitivním konci
  session (explicitní logout nebo `401` z refresh). 5xx/network chyba ho NEMAŽE.
- **Constant-time login je křehký — neměň pořadí.** `Verify` se musí volat na všech cestách
  a lock-check až po něm. Jakákoli "early return" před `Verify` (neexistuje user / je
  locked) prozradí časem, jestli nick existuje nebo je účet zamčený.
- **Login/Refresh běží mimo bus tx (`SkipTransaction`), ale z RŮZNÝCH důvodů.** Login: raw-pool
  counter zápis by se sám zablokoval uvnitř SQLite write tx. Refresh: theft cleanup
  (`DeleteByUserID`) musí auto-commitnout — kdyby běžel v tx, vrácený `AuthError` by ho
  rollbacknul a force-logout by neproběhl. Mechanika `SkipTransaction` → `/gk-bus`.
- **Brute-force counter žije přes raw pool, mimo tx.** Musí přežít rollback způsobený
  `AuthError`, který handler na konci vrací. Detaily raw-pool výjimek → `/gk-repositories`.

## Related
- Sousední skills: `/gk-permissions` (kdo smí co, permission stringy), `/gk-errors`
  (`AuthError` → `401`), `/gk-bus` (`SkipPermissionCheck` / `SkipTransaction`, middleware),
  `/gk-config` (`APP_JWT_SECRET`, expirace, `APP_COOKIE_SECURE`, rate limit), `/gk-commands`
  (struktura command handleru)
- Kód: `app/infrastructure/security/jwt.go`,
  `app/application/auth/command/{login,refresh_token,logout}.go`,
  `app/presentation/http/handler/auth.go`, `app/domain/token/`,
  FE: `assets/app.ts`, `assets/app-ui/Auth/{sessionHint,refresh,state,login,logout}.ts`
