---
layout: 'page'
uri: '/skills/gk-frontend-fetch'
position: 10
slug: 'skills-gk-frontend-fetch'
parent: 'skills-frontend'
navTitle: 'gk-frontend-fetch'
title: 'GK — Frontend data fetching'
description: 'FE data fetching — apiFetch vs authFetch, single-flight refresh + self-heal session, access token jen v paměti, discriminated-union ApiResponse, upload/download. Use when voláš z Vue backend API, řešíš proč ti request padá na 401, jak se obnovuje session, nebo jak vrátit data/chybu z fetch helperu.'
name: 'gk-frontend-fetch'
---

# GK — Frontend data fetching

Jak frontend (Vue SPA) volá backend API: jeden malý transport (`apiFetch`), jeho chráněná varianta s auto-refreshem (`authFetch`), a session, která se sama uzdraví po výpadku.

## What & when

- Sáhni sem, když: voláš z Vue komponenty/composable nějaký `/api/v1/...` endpoint, nevíš jestli `apiFetch` nebo `authFetch`, řešíš „proč mi request spadl na 401 a co se děje s tokenem", nebo přidáváš upload/download souboru.
- NEtýká se: tvar chybové odpovědi z backendu → na který HTTP status se mapuje (`/gk-errors`), ani jak backend login/refresh/logout a rotace tokenů funguje uvnitř (`/gk-auth`). Permission helpery (`hasPermission`…) sem patří jen okrajově — řídí UI, autoritativní je backend (`/gk-permissions`). Kam soubor patří a FE konvence řeší `/gk-frontend-ui`.

## For non-tech / juniors

Frontend a backend jsou dva oddělené programy; mluví spolu přes HTTP requesty (pošli data, dostaň odpověď). „Fetch helper" je naše obálka nad prohlížečovým `fetch`, aby se to volalo na jeden řádek a odpověď měla pořád stejný tvar.

Po přihlášení dostane frontend **access token** — krátkodobou propustku, kterou přikládá ke každému requestu. Token schválně **držíme jen v paměti** (JS proměnná), ne v `localStorage`: kdyby útočník propašoval do stránky cizí skript (XSS), z paměti běžící aplikace ho tak snadno nevytáhne. Cena: po tvrdém refreshi stránky je paměť prázdná — proto existuje „refresh", co propustku tiše obnoví z HttpOnly cookie (tu zase nevidí JS).

Access token brzo vyprší. Aplikace ho proto sama vyměňuje na pozadí 30 s před koncem. A když to volání jednou selže kvůli krátkému výpadku sítě/serveru, **session se neodhlásí** — počká a zkusí to znovu (self-heal). Odhlásí tě jen definitivní „tahle propustka už neplatí" (401) nebo když klikneš na Logout.

## How it works

Dvě vrstvy. **Fetch** (`assets/app-ui/Fetch/`) = surový transport bez retry. **Auth** (`assets/app-ui/Auth/`) = session, refresh a chráněná varianta.

**`apiFetch<TData, TError>(method, url, options)`** (`Fetch/apiFetch.ts`) — pošle JSON, vrátí `ApiResponse`. Přes `buildAuthHeaders` (`Fetch/buildHeaders.ts`) **přiloží `Authorization: Bearer`, kdykoli je v paměti token** — takže to není „bez auth", jen to **nemá refresh/retry**. Používej ho na public endpointy (`/health`) a interně ho volají i auth endpointy samotné (login/refresh/logout).

**`authFetch<TData, TError>(method, url, options)`** (`Auth/authFetch.ts`) — `apiFetch` + **jednorázový** retry na 401: zavolá `refresh()`, a když uspěje, **zopakuje request jednou**. Když refresh vrátí false, vrátí původní 401 (žádná smyčka). `/api/v1/auth/*` se schválně přeskakuje (login 401 = špatné heslo, refresh by se zacyklil, logout je one-shot). **Pro každý chráněný endpoint používej `authFetch`.**

**Access token** (`Fetch/accessToken.ts`) — jediná modulová proměnná `let accessToken`, `get/setAccessToken`. Jen v paměti, XSS-resistentní. Po hard-refreshi je prázdná → obnoví ji bootstrap.

**`ApiResponse<TData, TError>`** (`Fetch/types/ApiResponse.ts`) — discriminated union (rozlišená podle `success`):
```typescript
const r = await authFetch<UserProfile, ValidationError>('GET', '/api/v1/profile');
if (r.success === true)  { r.data; }   // ApiSuccess<TData>: { success:true,  status, data }
if (r.success === false) { r.data; }   // ApiError<TError>:   { success:false, status, data }
```
`parseResponse` (`Fetch/parseResponse.ts`) staví union podle `response.ok` — s výjimkou 2xx s nenaparsovatelným tělem, které vrací jako failure (`{ message: 'Malformed response body (status <status>)' }`), ne fake success; 2xx s prázdným tělem je success s `data: null`. Chybí-li JSON tělo u chybového statusu, doplní `{ message: 'Error <status>' }`. Síťovou/transportní chybu vrací `apiFetch` taky přes union: `{ success: false, status: 0 }` (nikdy nehodí výjimku). Default `TError` je `{ message: string }`. Default `TError` je `{ message: string }`.

**Single-flight refresh** žije v `refresh()` (`Auth/refresh.ts`), v `inFlight` guardu — **ne** v authFetch. Bootstrap, časovač 30 s před expirací, jeho retry i 401-retry z authFetch **sdílí jednu rotaci** cookie. Je to **bezpečnostní vlastnost**: paralelní rotace téže cookie backend (compare-and-swap nad `used_at`) vyhodnotí jako krádež tokenu a session natvrdo odhlásí.

**Self-heal** — `runRefresh()` / `onTransientFailure()` v `refresh.ts`:

| Výsledek `POST /api/v1/auth/refresh` | Reakce |
|---|---|
| 200 + validní tělo | nová session, `setAccessToken`, naplán další refresh |
| **401** (definitivní) | `clearSessionHint()` + `clearAuth()` → odhlášení |
| 200 malformed / 5xx / network error | **transient**: pokud `isAuthenticated && retries < 5` → jittered backoff (2 s base, ±50 %), session zůstane; jinak `clearAuth()` ale **hint zůstane** |

Asymetrie hintu JE ten self-heal: `clearAuth` (`Auth/state.ts`) schválně **nemaže** `gk_session` cookie (`Auth/sessionHint.ts`), takže další načtení stránky může refresh zkusit znovu. Hint se zruší jen při logoutu a 401.

**Bootstrap** (`assets/app.ts` → `bootstrap()`) — při hard-refreshi: pokud `hasSessionHint() === true`, zavolá `await refresh()` ještě před mountem routeru, takže se session tiše obnoví z cookie. `gk_session=1` je čitelná cookie vedle HttpOnly refresh cookie (JS HttpOnly nevidí) — ušetří zbytečný 401, když session zjevně není.

**Upload / download** (`Fetch/apiUpload.ts`, `Fetch/apiDownload.ts`) — `apiUpload(url, formData, onProgress?)` běží přes `XMLHttpRequest` (kvůli progress eventům), `apiDownload(url, fallbackFilename)` stáhne Blob a spustí browser dialog (filename z `Content-Disposition`, fallback parametr). Oba přikládají token přes `buildAuthHeaders`, ale jsou ve Fetch vrstvě — **bez 401 refreshe**.

## Recipe

### Recipe: zavolat chráněný endpoint z komponenty
1. Importuj: `import { authFetch } from '@/app-ui/Auth';`
2. Zavolej s typy dat i chyby: `const r = await authFetch<UserList, ValidationError>('GET', '/api/v1/users');`
3. Větvi přes `if (r.success === true)` / `=== false` (nikdy `if (!r…)` — viz CLAUDE.md FE pravidla).
4. Chybu napoj na formulářové pole: backend keyuje chyby podle pole → `errors.value = r.data;` (detail v `/gk-errors`).

### Recipe: public endpoint (bez nutnosti session)
1. `import { apiFetch } from '@/app-ui/Fetch';`
2. `const r = await apiFetch<{ status: string }>('GET', '/health');` — token se přiloží jen pokud existuje, ale na 401 se NEretrí. (Typ si napiš inline/vlastní — `/health` je infra-only a schválně stojí mimo tsgen, žádný generovaný `HealthResponse` neexistuje.)

### Recipe: upload s progressem
1. `import { apiUpload } from '@/app-ui/Fetch';`
2. `await apiUpload<Result>('/api/v1/files', formData, (s) => { s.percent; s.loaded; s.total; });`

## Invariants & pitfalls

- **Access token jen v paměti.** Nikdy ho neukládej do `localStorage`/`sessionStorage`/cookie. Jediný úložný bod je `Fetch/accessToken.ts`.
- **Chráněný endpoint → `authFetch`, ne `apiFetch`.** `apiFetch` token sice přiloží, ale po expiraci nezavolá refresh → request prostě skončí chybou 401.
- **`apiUpload` / `apiDownload` neretrují na 401.** Jsou ve Fetch vrstvě. Pokud token mezitím vypršel, chráněný upload/download selže — případně si nejdřív vynuť `await refresh()`.
- **authFetch retry je jednorázový.** Refresh uspěje → jeden opakovaný request; refresh false → původní 401. Nezacyklí se. `/api/v1/auth/*` se neretrí vůbec.
- **Single-flight = bezpečnost, ne optimalizace.** Nikdy nerotuj refresh cookie paralelně (vlastní souběžné `refresh()` mimo `inFlight` guard) — backend to vyhodnotí jako krádež tokenu a odhlásí.
- **Hint mažou jen logout a 401.** Při transientním selhání (5xx/offline) `clearAuth` hint NECHÁVÁ, aby další load mohl self-healnout. Nemaž `gk_session` při dočasné chybě.
- **Cross-tab souběh je roadmap trade-off.** Jitter jen de-synchronizuje retry napříč taby; plná koordinace mezi taby zatím není hotová — neprezentuj ji jako shipped.
- **Žádná FE validace.** Tvary `TError` jen typují odpověď serveru; autoritativní validace je vždy na backendu (CLAUDE.md).

## Related

- Skills: `/gk-errors` (tvar `data` v `ApiError` ↔ HTTP status mapping na backendu), `/gk-auth` (backend dvou-tokenový login/refresh/logout, rotace + theft detection, kterou single-flight chrání), `/gk-permissions` (zdroj pravdy pro `hasPermission`), `/gk-frontend-ui` (struktura FE, kam komponenta/util patří)
- Kód: `assets/app-ui/Fetch/` (`apiFetch`, `parseResponse`, `buildHeaders`, `accessToken`, `apiUpload`, `apiDownload`, `types/ApiResponse`), `assets/app-ui/Auth/` (`authFetch`, `refresh`, `login`, `logout`, `state`, `sessionHint`, `useAuth`), `assets/app.ts` (`bootstrap`)
