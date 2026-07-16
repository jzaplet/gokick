---
layout: 'page'
uri: '/skills/gk-architecture'
position: 30
slug: 'skills-gk-architecture'
parent: 'skills-start'
navTitle: 'gk-architecture'
title: 'GK — Architecture (DDD 4 vrstvy + CQRS)'
description: 'Mentální model celého gokicku — DDD 4 vrstvy, CQRS, pravidla závislostí a bounded kontexty vynucené go-arch-lintem. Use when potřebuješ pochopit, kam nový kód patří, proč nějaký import neprojde, nebo jak vrstvy mezi sebou mluví.'
name: 'gk-architecture'
---

# GK — Architecture (DDD 4 vrstvy + CQRS)

Jednou větou: gokick je rozdělený na čtyři vrstvy s přísnými pravidly, kdo koho
smí importovat, a `go-arch-lint` ta pravidla hlídá v `make lint` / `make arch-check` (a v CI).

## What & when

- Sáhni sem, když nevíš, **kam nový kód patří** (entita? handler? repozitář?),
  proč ti `make arch-check` spadl na importu, nebo jak spolu vrstvy komunikují.
- Tohle je rozcestník / mentální mapa. Detail konkrétní vrstvy řeší sousední
  skills (viz `## Related`) — sem chodíš pro celkový obrázek, ne pro recept na
  jeden command.

## For non-tech / juniors

Představ si firmu se čtyřmi patry a pravidlem „nižší patro nesmí volat nahoru":

- **Domain** (přízemí) — čistá pravidla businessu: co je uživatel, jaký smí mít
  nickname. Nezná databázi ani web, nezávisí na ničem.
- **Application** — dirigent: vezme požadavek („vytvoř uživatele"), zkontroluje
  oprávnění, spustí transakci, zavolá pravidla z přízemí.
- **Infrastructure** — sklad a nářadí: SQLite databáze, hashování hesel, JWT.
  Plní rozhraní, která si přízemí definovalo.
- **Presentation** — recepce: HTTP a CLI. Přijme request, předá ho dirigentovi,
  vrátí odpověď.

Smysl: přízemí (business pravidla) se nikdy nemusí měnit, když vyměníš databázi
nebo web framework. A protože každé patro smí volat jen ta správná, kód se
nezamotá do sebe. **CQRS** znamená, že čtení (query) a zápis (command) jsou dvě
oddělené cesty s jiným chováním (zápis má transakci a audit, čtení ne).

## How it works

**Směr závislostí** (`docs/framework/architecture.md`):

```
presentation --> application --> domain <-- infrastructure
     |                                        ^
     +----------------------------------------+
```

Domain nezávisí na ničem (jen stdlib + `uuid`). Každá vyšší vrstva smí importovat
jen ty pod sebou. Plná matice závislostí je v `.go-arch-lint.yml` (`mayDependOn` u každé komponenty).

**Vrstvy a složky:**

| Vrstva | Složka | Obsah |
|---|---|---|
| Domain | `app/domain/` | entity, value objects, repository interfaces, eventy, errory |
| Application | `app/application/` | CQRS busy + command/query/event handlery (po doménách) |
| Infrastructure | `app/infrastructure/` | SQLite repo, bcrypt, JWT, config, Wire DI |
| Presentation | `app/presentation/` | HTTP handlery + middleware, Cobra CLI |

**Bounded kontexty** = samostatné balíčky uvnitř `domain/` (`app/domain/user/`,
`app/domain/token/`, `app/domain/run/`). Sdílené typy žijí v `app/domain/shared/`
(`AuthClaims`, error typy, service interfaces). Jeden kontext **nesmí** importovat
druhý — komunikace jde přes QueryBus nebo domain eventy.

**CQRS busy** (tři user-facing + operator-trusted `SystemCommandBus` pro CLI create-*/seed, každý s vlastním řetězcem middleware). Řetězce se skládají v `app/application/bus/middleware/base.go` (`busmw.BaseChain` = `Recovery → Logging → Authorize → Tenant`; `busmw.CommandChain` na něj navěsí write-side zbytek) — DI providery v `app/infrastructure/di/container_provider.go` (`provideCommandBus`, `provideQueryBus`, `provideEventBus`) je jen volají:

| Bus | Řetězec | K čemu |
|---|---|---|
| `CommandBus` | Recovery → Logging → Authorize → Tenant → **Audit → RunDispatcher → DispatchEvents → Transaction** | zápisy |
| `SystemCommandBus` | Recovery → Logging → **Audit → RunDispatcher → DispatchEvents → Transaction** | CLI zápisy (bez Authorize/Tenant) |
| `QueryBus` | Recovery → Logging → Authorize → Tenant | čtení |
| `EventBus` | Recovery → Logging | side-effects po commitu |

Audit je **vně** transakce (`busmw.CommandChain`), aby bezpečnostní eventy
(login_failed, account_locked) přežily i rollback business transakce.

**Vynucení lintem** (`.go-arch-lint.yml`, `workdir: app`): doména je rozdělená na
komponenty `domain_shared`, `domain_user`, `domain_token`, `domain_run`, `domain_tenant`. Jen
`domain_shared` je v `commonComponents` (dostupná všem). Ostatní kontexty common
**nejsou**, takže import jednoho z druhého je porušení pravidla komponenty — přesně
to vynucuje „no cross-context imports". Broad-glob komponenty (`application/**`,
`presentation/http/handler/**`) pokryjí nové subbalíčky automaticky; bounded
kontexty jsou vyjmenované ručně.

## Recipe

### Recipe: kam dát nový kód

1. Nová **business entita / pravidlo** → `app/domain/<context>/`.
2. **Uložení do DB** → `app/infrastructure/sqlite/<context>/` (implementuje
   interface z domény).
3. **Operace** (use-case) → `app/application/<context>/command/` (zápis) nebo
   `.../query/` (čtení) — handler musí deklarovat permission (viz níže).
4. **HTTP vstup** → `app/presentation/http/handler/` + route v
   `app/presentation/http/server/`.
5. **Propojení** → Wire provider v `app/infrastructure/di/container_provider.go`,
   pak `make di && make arch-check`.

### Recipe: přidat nový bounded kontext (např. `order`)

1. Vytvoř `app/domain/order/` + `app/infrastructure/sqlite/order/`.
2. V `.go-arch-lint.yml`: přidej komponentu `domain_order` (`in: domain/order/**`),
   přidej `infrastructure/sqlite/order/**` do `sqlite_repos`.
3. Povol `domain_order` v `mayDependOn` u každého konzumenta (`application`,
   `sqlite_repos`, `testfx`, případně `handler`, `worker`).
4. `make arch-check` ověří, že nikdo kontext neimportuje načerno.

## Invariants & pitfalls

- **Domain neimportuje nic** než stdlib + `uuid`. Žádný `database`, `security`,
  `bus` v doméně.
- **Žádné cross-context importy.** `domain/user/` nesmí importovat `domain/token/`.
  Sdílené typy patří do `domain/shared/`.
- **Command/query přes bus, nikdy přímo.** HTTP handler posílá přes `bus.Dispatch` / `bus.DispatchVoid` (command) a `bus.Query` (query) — typované na `*CommandBus` / `*QueryBus`, takže záměna busu neprojde kompilací; bus dodá recovery, logging, autorizaci, tenant a transakci.
- **Každý command/query deklaruje permission** — buď `Permissioned`
  (`RequiredPermission() string`) nebo `SkipPermission` (`SkipPermissionCheck()`),
  oba z `app/domain/shared/permission.go`. Když chybí obojí, `AuthorizeMiddleware`
  vrátí error (chrání proti zapomenuté deklaraci).
- **`r.Conn(ctx)` v repozitářích**, ne `r.DB.DB()` — kvůli transparentní účasti
  v transakci. Výjimka (raw pool, dokumentovaná v komentáři metody): zápisy, co
  musí přežít rollback — `RecordFailedLogin`/`ResetFailedLogin`/`RecordLogin`
  (`app/infrastructure/sqlite/user/repository.go`) a audit
  (`app/infrastructure/sqlite/audit/repository.go`).
- **Context klíče: `type xxxKey struct{}` + `xxxKey{}`** — nový klíč do `context`
  (v `domain/shared`) se deklaruje jako prázdný typ a používá jako literál
  `xxxKey{}` v `context.WithValue` / `ctx.Value`. Ne přes pomocnou `var xxxKey =
  xxxKeyType{}` (delší, tváří se mutable). Jeden styl napříč balíčkem.
- **Eventy nesou jen primitivy** (string ID, timestamp), nikdy celé entity.
- **Nový kontext = editace `.go-arch-lint.yml`.** Není tam `domain/**` catch-all
  schválně — bez ručního zápisu komponenty `make arch-check` spadne.

## Related

- Sousední skills: `/gk-feature` (přidání featury end-to-end přes vrstvy),
  `/gk-bus` (CQRS busy + middleware chain + dispatch), `/gk-entities` (entity a
  value objects v doméně), `/gk-domain-events` (eventy mezi kontexty).
- Docs: `/framework/architecture`
- Kód: `.go-arch-lint.yml`, `app/domain/`, `app/application/bus/middleware/`,
  `app/infrastructure/di/container_provider.go`, `app/domain/shared/permission.go`
