---
layout: 'page'
uri: '/skills/gk-permissions'
position: 20
slug: 'skills-gk-permissions'
parent: 'skills-auth'
navTitle: 'gk-permissions'
title: 'GK — Permissions (oprávnění)'
description: 'Oprávnění end-to-end — kdo smí spustit který command/query a co vidí na frontendu, řízené jedním zdrojem pravdy odvozeným z kódu. Use when přidáváš chráněný endpoint, řešíš proč request padá na 403/401, schováváš UI podle role, nebo přidáváš novou permission.'
name: 'gk-permissions'
---

# GK — Permissions (oprávnění)

Kdo smí spustit kterou operaci a co uvidí na frontendu — jeden zdroj pravdy odvozený přímo z handlerů.

## What & when
- Sáhni sem, když: přidáváš chráněný command/query, request ti vrací **403** (špatná role) nebo **401** (chybí přihlášení), schováváš tlačítko/route podle role, nebo přidáváš novou permission.
- NEtýká se: jak vypadá samotný command/query handler (`/gk-commands`, `/gk-queries`), jak teče middleware chain (`/gk-bus`), ani mapování chyb na HTTP kódy (`/gk-errors`).

## For non-tech / juniors
Permission je řetězec jako `admin:users:create` — pojmenovaná „klíčenka" k jedné akci. Každá operace v aplikaci si u sebe napíše, jaký klíč k ní potřebuješ. Při startu aplikace projde všechny operace a sesbírá z nich seznam klíčů — **žádný ruční seznam permissions neexistuje**, je odvozený z kódu. Když přijde request, brána (middleware) se podívá na roli přihlášeného a rozhodne, jestli má potřebný klíč.

Pravidlo je jednoduché: role `admin` má všechny klíče. Každá jiná role má všechny **kromě těch, co začínají `admin:`**. Žádné jemnější přiřazování klíčů jednotlivým uživatelům zatím není.

Frontend dostane při loginu seznam klíčů pro svoji roli a podle něj jen **schovává UI** (tlačítka, stránky). To je kosmetika — skutečnou kontrolu dělá vždycky backend.

## How it works

**Formát:** `<doména>:<akce>`, např. `profile:read`, `auth:logout`, `admin:users:create`.

**Deklarace (backend).** Každý command/query MUSÍ implementovat jedno ze dvou rozhraní z `app/domain/shared/permission.go`:
```go
func (DeleteUserCommand) RequiredPermission() string { return "admin:users:delete" } // Permissioned
func (LoginCommand) SkipPermissionCheck() {}                                          // SkipPermission (veřejné)
```

**Kontrola (backend).** `AuthorizeMiddleware` (`app/application/bus/middleware/authorize.go`) běží v command i query bus:
1. `SkipPermission` → propustí. `Permissioned` → zavolá `PermissionChecker.Check(ctx, perm)`. Nic z toho → vrátí error (`must implement Permissioned or SkipPermission`) — pojistka proti zapomenuté deklaraci.
2. `PermissionChecker` (`app/infrastructure/security/permission.go`) přečte `AuthClaims` z kontextu (vkládá je JWT `AuthMiddleware`, `app/presentation/http/middleware/auth.go`). Žádné claims → `*shared.AuthError` (HTTP 401).
3. Zavolá `shared.IsPermissionAllowedForRole(perm, role)` — `admin` → vše; jinak vše kromě prefixu `admin:`. Nesedí → `*shared.PermissionError` (HTTP 403).

**Registry = seznam pro frontend.** `PermissionsRegistry` (`app/domain/shared/permissions_registry.go`) sesbírá `RequiredPermission()` ze všech handlerů (deduplikace + sort). Konkrétní seznam handlerů je ve Wire provideru `providePermissionsRegistry` v `app/infrastructure/di/container_provider.go` (aktuálně 9 položek). `reg.ForRole(role)` vrátí permissions pro roli — login (`app/presentation/http/handler/auth.go`) i `/profile` (`profile.go`) ho dají do `user.permissions` v response.

**Frontend = jediný enum.** `Permission` v `assets/app/Auth/enums/resources.ts` zrcadlí backend stringy (single source of truth na FE — nikde jinde se string literál nepíše). Helpery v `assets/app-ui/Auth/permissions.ts` (`hasPermission`, `hasAllPermissions`, `hasAnyPermission`, `isAdmin`) berou typovaný `Permission` a čtou `user.permissions` ze stavu; `admin` je vždy `true`. Vystaveno přes `useAuth()` (`assets/app-ui/Auth/useAuth.ts`).

**Route guard.** `assets/router/meta.ts` typuje `meta.requiresAuth: boolean` (povinné) a `meta.requiresPermission?: Permission`. `assets/router/authGuard.ts`: nepřihlášený na `requiresAuth` route → redirect na login; chybějící permission → toast + redirect domů. Routy: `assets/router/routes.ts`.

## Recipe

### Recipe: přidat novou permission
1. Na command/query přidej `func (X) RequiredPermission() string { return "reports:export" }` (nebo `SkipPermissionCheck()` pro veřejné).
2. Přidej instanci do slice v `providePermissionsRegistry` (`container_provider.go`) — jinak ji frontend v login response nedostane.
3. Přidej zrcadlovou položku do `Permission` v `assets/app/Auth/enums/resources.ts` (např. `ReportsExport: 'reports:export'`).
4. `make di && make test`.

### Recipe: chránit frontend route / UI podle permission
1. Route: do `routes.ts` přidej `meta: { requiresAuth: true, requiresPermission: Permission.ReportsExport }`.
2. UI prvek (tlačítko, sekce): `const { hasPermission } = useAuth()` → `v-if="hasPermission(Permission.ReportsExport)"`. Nikdy nepiš string literál.

## Invariants & pitfalls
- **Každý command/query deklaruje permission.** Chybí-li `Permissioned` i `SkipPermission`, bus vrátí runtime error — není to volitelné.
- **Registry je jediný zdroj pravdy a musí se ručně doplnit.** Nový `Permissioned` handler, který nepřidáš do `providePermissionsRegistry`, je sice chráněný backendem, ale frontend o permission neví → UI se chová, jako bys ji neměl. Druhá konfigurace neexistuje.
- **Žádné raw permission stringy na FE.** Každá reference jde přes `Permission` enum (`resources.ts`). Literál `'admin:users:read'` v `.vue`/`.ts` je zakázaný — enum je typovaný, překlep = compile-time chyba.
- **FE schovává, BE rozhoduje.** `hasPermission` je jen UX; autoritativní kontrola je vždy `AuthorizeMiddleware` na backendu. Nikdy nespoléhej na FE jako na bezpečnostní hranici.
- **Jen role-based, žádné per-user granty.** `IsPermissionAllowedForRole` zná jen `admin` (vše) a ostatní (vše kromě `admin:*`). Jemnější model zatím není.
- **`meta.requiresAuth` je povinné na každé routě** (typ `AppRoute`) — žádná implicitní „public". Stejně jako BE pravidlo `Permissioned`/`SkipPermission`.

## Related
- Sousední skills: `/gk-bus` (middleware chain), `/gk-commands` + `/gk-queries` (struktura handlerů), `/gk-errors` (401/403 mapování), `/gk-feature` (přidání featury end-to-end)
- Kód (BE): `app/domain/shared/permission.go`, `app/domain/shared/permissions_registry.go`, `app/application/bus/middleware/authorize.go`, `app/infrastructure/security/permission.go`, `app/infrastructure/di/container_provider.go` (`providePermissionsRegistry`)
- Kód (FE): `assets/app/Auth/enums/resources.ts`, `assets/app-ui/Auth/permissions.ts`, `assets/app-ui/Auth/useAuth.ts`, `assets/router/{meta,routes,authGuard}.ts`
