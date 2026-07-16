---
layout: 'page'
uri: '/skills/gk-frontend-ui'
position: 30
slug: 'skills-gk-frontend-ui'
parent: 'skills-frontend'
navTitle: 'gk-frontend-ui'
title: 'GK — Frontend struktura & konvence'
description: 'Struktura a konvence frontendu — sdílené app-ui komponenty + composables, doménová organizace app/, router s guardy a maximálně přísný TypeScript/ESLint/Tailwind. Use when přidáváš nebo upravuješ Vue komponentu/view, řešíš kam soubor patří, importuješ fetch/auth/toast utilitu, registruješ route nebo nevíš, proč ti lint/tsc na něco nadává.'
name: 'gk-frontend-ui'
---

# GK — Frontend struktura & konvence

Vue 3 SPA v `assets/`, doménově organizované, s vrstvou sdílených UI utilit (`app-ui/`) a maximálně přísným TypeScriptem. Buildí se Vite a embeduje do Go binárky.

## What & when

- Sáhni sem, když **píšeš nebo měníš cokoli ve `assets/`** — komponentu, view, formulář, route — a potřebuješ vědět, **kam soubor patří** a **odkud importovat** hotové utility (`apiFetch`, `authFetch`, `useAuth`, `useToast`, `ConfirmModal`…).
- Sáhni sem, když **lint nebo `vue-tsc` padá** na věci, co jinde v JS projektech projdou (přístup přes tečku k dynamickému klíči, implicitní ne-boolean podmínka, `interface` místo `type`, `==` místo `===`) — pravidla jsou tu schválně tvrdší.
- **NEtýká se** detailů formulářů a chybových odpovědí (to je vlastní téma — viz `Related`), permission stringů a `Permission` enumu (`/gk-permissions`), ani backendu.

## For non-tech / juniors

Frontend je **jedna stránka v prohlížeči** (SPA — single-page application), která si data tahá z backendu přes HTTP. Místo aby se každá podstránka načítala znovu ze serveru, router (orientační plánek „URL → komponenta") jen překresluje obsah.

Kód je rozdělený na dvě poloviny:
- **`assets/app/<Doména>/`** — věci specifické pro konkrétní oblast aplikace (Admin, Auth, Profile…). Sem patří obrazovky a jejich formuláře.
- **`assets/app-ui/`** — **sdílené „lego kostky"**: tlačítko, modal, toast (vyskakovací notifikace), a hotové funkce na volání API. Píšeš je jednou, používáš všude. Když potřebuješ tlačítko, neděláš nové — vezmeš `Button` odsud.

„Maximálně přísný TypeScript" znamená, že nástroje (lint + typová kontrola) tě **zastaví už při psaní**, když uděláš riskantní věc (sáhneš na klíč, co nemusí existovat; porovnáš nejednoznačně). Cíl: chyby vyskočí na obrazovce u tebe, ne až u uživatele.

## How it works

**Doménová organizace** (`assets/app/<Domain>/`, reálné domény: `Admin`, `Auth`, `Dashboard`, `Home`, `Layout`, `Platform`, `Profile`):
- **`Views/`** — routované obrazovky, jsou to **orchestrátory**: poskládají layout, namountují pár komponent, předají props. Business logika sem nepatří. Příklad: `app/Admin/Views/AdminUsersView.vue`.
- **`Components/`** — doménové, samostatné kusy: formuláře (`UserForm.vue`), řádky gridu (`AdminUserRow.vue`), karty. Tabulku samotnou doména nevlastní — skládá ji sdílený `app-ui/DataGrid`, kterému doména dodá jen `<tr>` do slotu `#rows`.
- **`types/`** — typové definice, jeden typ na soubor. Wire DTO typy (`AdminUser.ts`, `UserFormData.ts`) generuje z Go structů `make ts-gen` (tsgen, hlavička `DO NOT EDIT` — needituj ručně, uprav Go struct); ručně píšeš jen FE-lokální typy (`UserFormErrors.ts`).

**Sdílené `app-ui/`** — generic, znovupoužitelné napříč doménami. Klíčové vstupní body (importuj z barrelu, ne z konkrétního souboru):
- **`@/app-ui/Fetch`** — `apiFetch` (public endpoint bez JWT), `apiUpload`, `apiDownload`. Návrat je `ApiResponse<TData, TError>` = sjednocení `{ success: true, status, data }` / `{ success: false, status, data }` (`app-ui/Fetch/types/`).
- **`@/app-ui/Auth`** — `authFetch` (protected endpoint, na 401 sám zavolá `refresh()` a request zopakuje), `useAuth` (reaktivní session: `user`, `isAuthenticated`, `login/logout/refresh`, permission helpery), a re-exportované `hasPermission`, `hasRole`, `isAdmin`, `isSuperAdmin`, `hasAllPermissions`, `hasAnyPermission` pro použití mimo `<script setup>`.
- **`@/app-ui/Toast/useToast`** — `success/error/info/warning/clear` notifikace.
- **`@/app-ui/ClickOutside/useClickOutside`** — composable: zavolá callback při kliku mimo element.
- **UI komponenty**: `Inputs/` (`Input`, `Select`), `Buttons/Button`, `Modals/ConfirmModal`, `Dropdown/`, `Alerts/ErrorAlert`, `Loading/Spinner`, `Icons/` (SVG).

> **Composable** = sdílená funkce začínající `use…`, která zabaluje kus reaktivní logiky (stav + lifecycle) k znovupoužití mezi komponentami. V tomhle projektu nahrazuje to, k čemu by jinde sloužila třída.

**Router** (`assets/router/`, ne jediný `router.ts`):
- `routes.ts` — pole `AppRoute[]`. **Každá route MUSÍ mít `meta.requiresAuth: true|false`** — typ `AppRoute` (`router/meta.ts`) to vynutí, neexistuje implicitní „public". Chráněné admin routy přidají `requiresPermission: Permission.X` (z enumu, ne string).
- `authGuard.ts` — `router.beforeEach` guard: bez session na `requiresAuth` route → redirect na `login`; chybí permission → toast + redirect na `home`.
- `index.ts` — sestaví router a navěsí guard.
- Session restore: `assets/app.ts` `bootstrap()` zavolá `refresh()` (jen když `hasSessionHint()`) **před mountem** — po hard refreshi se session tiše obnoví z cookie.

**Tooling** (`tsconfig.json`, `eslint.config.ts`): TypeScript běží na `strict` + navíc `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `noPropertyAccessFromIndexSignature`, `verbatimModuleSyntax`. ESLint = `strictTypeChecked` + `stylisticTypeChecked` + Vue `recommended-error` + Stylistic (4 mezery, single quotes, semi; nahrazuje Prettier). Spouští se přes `make lint` / `make format`.

## Recipe: přidat novou obrazovku (view)

1. **Doména** — najdi/založ `assets/app/<Domain>/`. View dej do `Views/`, jeho formuláře/tabulky do `Components/`, typy do `types/` (jeden typ = jeden soubor).
2. **View je orchestrátor** — jen layout + mount komponent + předání props. Logiku (fetch, validace) drž v komponentách, ne ve view.
3. **Komponenta** — `<script setup lang="ts">`, props přes `defineProps<Type>()` s destrukturací, typy přes `type` (ne `interface`). Importuj UI z `@/app-ui/...`, vždy přes `@/` alias.
4. **Data** — protected endpoint → `authFetch<Data, Errors>(...)` z `@/app-ui/Auth`; public → `apiFetch` z `@/app-ui/Fetch`. Větvi přes `result.success === true/false`.
5. **Route** — přidej záznam do `assets/router/routes.ts` s `meta.requiresAuth`; pokud chráněná, `requiresPermission: Permission.X` z `@/app/Auth/enums/resources`.
6. **`make lint && make format`**, pak `make test`.

## Invariants & pitfalls

- **Views jsou orchestrátory.** Business logika nepatří do `Views/` — patří do `<Domain>/Components/`.
- **Vždy `@/` alias, nikdy relativní cesty** (`../../`). Vynuceno konvencí v `CLAUDE.md`.
- **Žádná frontend validace.** Backend je autoritativní; chyby přicházejí v odpovědi a mapují se na pole formuláře.
- **Maximální přísnost — tohle ti lint/tsc zařízne:** `interface` pro běžné typy (jen `type`; `interface` výjimečně u module augmentation, viz `router/meta.ts`), `obj.key` u index signatury (musí `obj['key']`), implicitní ne-boolean podmínka (`strict-boolean-expressions`), `==`/`!=` (jen `===`/`!==`), `console.*`/`debugger`/`alert`, soubor přes 300 řádků / vnoření hlubší než 4 (`max-lines`/`max-depth`). **Konvence z `CLAUDE.md` (lint je zatím nehlídá, no-as je plánovaný ratchet):** `as` cast (použij generika; runtime-validační follow-up srazil tolerovanou zónu na `parseResponse.ts` + `useToast.ts` — 3 zbylé casty, viz roadmap ⑥), `function` keyword a `class` (jen arrow funkce / composables), `.forEach()` (použij `for...of`), `if (!x)` → `if (x === false)` / `if (x === null)`.
- **Každá route deklaruje `meta.requiresAuth`** — bez něj neprojde typ `AppRoute`. Zrcadlí backendové `Permissioned` / `SkipPermission`.
- **Žádné hard-coded permission stringy ve `assets/`** — vždy `Permission` enum z `@/app/Auth/enums/resources` (jeden zdroj pravdy, zrcadlí backend). Detail → `/gk-permissions`.
- **Tailwind v template:** dlouhé seznamy tříd (5+ utilit) přes `:class="[...]"` array, krátké (1–4) jako plain `class="..."`. SVG `d` atribut lámej po ~120 znacích.
- **`app-ui/` má výjimku** na single-word názvy komponent (`Button`, `Input`) — jinde `multi-word-component-names`. Importuj z barrel `index.ts`, ne z vnitřního souboru.

## Related

- `/gk-permissions` — `Permission` enum, skrývání UI podle role, 401/403.
- `/gk-architecture` — vrstvy backendu a proč FE zrcadlí backendová pravidla.
- Kód: `assets/app-ui/` (sdílené komponenty + composables), `assets/app/<Domain>/{Views,Components,types}/`, `assets/router/{routes,authGuard,meta,index}.ts`, `assets/app/Auth/enums/resources.ts`, `tsconfig.json`, `eslint.config.ts`.
