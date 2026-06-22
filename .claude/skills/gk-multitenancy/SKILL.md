---
name: gk-multitenancy
description: Zapínatelný row-level multitenancy + platformní rovina (superadmin). Use when zapínáš/ladíš tenant izolaci, přidáváš tenant-owned tabulku, řešíš proč dotaz leakuje cizí tenant, nebo zakládáš tenanty/superadmina z CLI.
layout: 'page'
uri: '/skills/gk-multitenancy'
position: 50
slug: 'skills-gk-multitenancy'
parent: 'skills-auth'
navTitle: 'gk-multitenancy'
title: 'GK — Multitenancy'
---

# GK — Multitenancy

Zapínatelný **row-level** multitenancy (jeden přepínač) + platformní rovina nad tenanty pro autory aplikace. Vypnuto = dnešní single-tenant chování, nula rozdílu.

## What & when

- Sáhni sem, když: zapínáš/ladíš tenant izolaci, **přidáváš tenant-owned tabulku** (musí být „born scoped"), řešíš „proč mi dotaz vrací cizí tenant", zakládáš tenanty / superadmina, nebo nechápeš `APP_MULTITENANCY`.
- Kdo smí co (permission stringy, FE enum) řeší `/gk-permissions`; tx-aware datová vrstva `/gk-repositories`. Tenhle skill je o **tenant scopování** nad nimi.

## For non-tech / juniors

**Tenant** = „čí jsou ta data" (workspace / zákazník), oddělené od **user** = „kdo jsi". Multitenancy znamená, že v jedné databázi žije víc tenantů a každý vidí **jen svoje** data. gokick to dělá **row-level**: každý řádek owned tabulky nese `tenant_id` a dotazy filtrují podle něj.

Bez ORM (gokick píše SQL ručně) **neexistuje** automatické „přihoď `WHERE tenant_id`". Proto: **resolver tenant dodá** (z přihlášení), **repo ho aplikuje** (do dotazu), a **conformance test v CI hlídá**, že na to nikdo nezapomněl. Nad tenanty je **superadmin** (autor aplikace), který vidí napříč všemi — opak izolace.

## How it works

**Přepínač.** `APP_MULTITENANCY` (default `false`) → `config.Multitenancy`. Vybírá jen **striktnost vynucení**, ne resoluci: fail-open (chybějící tenant → default) vs fail-closed (panika).

**Resoluce (data-driven).** JWT nese claim `tenant` (mint i verify v `app/infrastructure/security/jwt.go`; login/refresh ho razí z `u.TenantID`). `shared.TenantResolver` (port) + `security.DefaultTenantResolver` vrátí `claims.TenantID`, jinak `shared.DefaultTenantID`. `TenantMiddleware` (v `BaseChain`, hned za Authorize) ho uloží do ctx (`shared.ContextWithTenantID`) — pokrývá command i query bus.

**Aplikace.** `BaseRepository.Tenant(ctx)` (`app/infrastructure/sqlite/conn.go`) vrátí tenant z ctx; když chybí: **panika** v multitenant režimu (fail-closed), jinak `DefaultTenantID`. Repo ho dá do `WHERE … AND tenant_id=?` (viz `app/infrastructure/sqlite/user/repository.go`: `FindAll`/`FindAllActive`/`Update`/`Delete`).

**Conformance gate.** `app/infrastructure/sqlite/zz_tenant_test.go` AST-skenuje SQL string literály všech repo: dotaz nad **tenant-owned** tabulkou (`tenantOwnedTables`) musí mít `tenant_id` **NEBO** inline `/* tenant-scope-exempt: <důvod> */` marker; **neklasifikovaná** tabulka = FAIL. Padá v CI, ověřeno mutací.

**Worker propagace.** Worker obchází bus → `TenantMiddleware` se na job handlery nespustí. `jobs.tenant_id` se razí při enqueue (dispatcher z ctx) a worker ho v `runWithinTx` obnoví do ctx z claimnutého řádku před handlerem. `ClaimDue` zůstává globální drain (marker-exempt).

**Platformní rovina.** Role `superadmin` (nad admin/user) + `platform:*`. Žebřík v `shared.IsPermissionAllowedForRole`: superadmin → vše; `platform:*` brána sedí **mezi** superadmin a admin (admin platform nedostane). Cross-tenant dotazy (`FindAllAcrossTenants`, `FindAllWithUserCount`) nesou `/* tenant-scope-exempt: platform superadmin */`. Endpointy `GET /api/v1/platform/{stats,users,tenants}` + `PUT/DELETE /api/v1/platform/users/{id}`; FE sekce na `/platform/*` (jen superadmin). FE `permissions.ts` zrcadlí backend žebřík.

**Operator tooling.** Seed s MT on založí adminovi vlastní tenant (`APP_SEED_ADMIN_TENANT`, find-or-create); superadmin vždy v default tenantu. CLI: `create-tenant`, `create-superadmin`, `create-user --tenant-id/--tenant-name`.

## Recipe

### Recipe: zapnout multitenancy

1. `.env`: `APP_MULTITENANCY=true` (+ volitelně `APP_SEED_ADMIN_TENANT`, `APP_SEED_SUPERADMIN_PASSWORD`).
2. `./bin/app seed` — admin dostane vlastní tenant, superadmin (je-li heslo) jde do default.
3. Další uživatele zakládej s tenantem: `create-user … --tenant-name <nový>` nebo `--tenant-id <existující>` (id z `create-tenant`).

### Recipe: přidat tenant-owned tabulku

1. Migrace: sloupec `tenant_id TEXT NOT NULL REFERENCES tenants(id)` (SQLite neumí `ALTER ADD` FK → table-rebuild, viz `/gk-migrations`).
2. Repo: scopuj každý dotaz přes `r.Tenant(ctx)` (`WHERE … AND tenant_id=?`); na INSERT stampuj tenant z entity/ctx.
3. Gate: přidej tabulku do `tenantOwnedTables` v `zz_tenant_test.go`. Legitimní výjimky (identity/auth, control-plane) nesou inline `/* tenant-scope-exempt: <důvod> */`.
4. `make test` — gate musí projít.

## Invariants & pitfalls

- **Resolver dodá, repo aplikuje.** Žádné transparentní `WHERE` — proto je conformance gate povinný.
- **Každý dotaz na owned tabulku: `tenant_id` NEBO marker.** Jinak CI padne (neklasifikovaná tabulka = FAIL).
- **Tichý read-leak na slepém místě scanneru** (dynamicky stavěné SQL, JOIN, dotazy mimo skenované adresáře) je riziko SQLite fáze — transparentní vynucení dá až **Postgres RLS** (roadmap).
- **Vytváření obchází `r.Tenant`** — `Save` píše `tenant_id` explicitně z entity, takže enforcement při vytváření **nesedí na repo**, ale na vstupní cestě: HTTP bere tenant z přihlášeného admina (TenantMiddleware), CLI ho vyžaduje přes `--tenant-id/--tenant-name` (s MT on).
- **Superadmin se nezakládá přes admin API.** `CreateUser`/`UpdateUser` roli `superadmin` odmítnou (anti-eskalace) a admin repo dotazy vylučují `role='superadmin'` — superadmina dělá jen `create-superadmin` / seed.

## Related

- `/gk-permissions` (role/permission, FE enum) · `/gk-repositories` (tx-aware datová vrstva) · `/gk-jobs` (worker) · `/gk-auth` (JWT) · `/gk-migrations` (table-rebuild)
- Docs: [Installation](/framework/installation) (CLI), [Roadmap](/framework/gokick-roadmap)
- Kód: `app/domain/{tenant,shared}/`, `app/infrastructure/sqlite/{conn.go,zz_tenant_test.go}`, `app/application/platform/`, `assets/app/Platform/`
