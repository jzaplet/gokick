---
layout: 'page'
uri: '/skills/gk-di'
position: 50
slug: 'skills-gk-di'
parent: 'skills-data'
navTitle: 'gk-di'
title: 'GK — Wire compile-time DI'
description: 'Wire compile-time DI — kde se registrují providery, jak se interface naváže na implementaci přes wire.Bind a kdy spustit make di. Use when přidáváš službu/constructor do DI, vážeš doménový interface na konkrétní typ, změnil jsi signaturu konstruktoru, nebo ti build hlásí chybu ve wire_gen.go.'
name: 'gk-di'
---

# GK — Wire compile-time DI

Skládání závislostí (kdo dostane co do konstruktoru) se generuje **při kompilaci** z explicitního seznamu — žádná reflexe, žádný runtime kontejner.

## What & when

- Sáhni sem, když: přidáváš novou službu/constructor do DI, vážeš doménový interface na konkrétní implementaci, změnil jsi signaturu nějakého `NewX(...)` a něco přestalo jít zkompilovat, nebo build hlásí chybu ukazující na `wire_gen.go`.
- NEtýká se: jak busy zpracovávají command/query (`/gk-bus`), odkud se berou hodnoty z `.env` (`/gk-config`), ani pravidel vrstev (`/gk-architecture`). DI je jen **lepidlo**, které tyhle věci propojí.

## For non-tech / juniors

Aplikace je složená z desítek malých dílků (databáze, hashování hesel, JWT, HTTP handlery…). Každý dílek ke svému vzniku potřebuje jiné dílky — handler potřebuje repozitář, repozitář potřebuje připojení k DB. **DI (dependency injection)** je způsob, jak tyhle dílky poskládat dohromady, aniž by si je každý sháněl sám.

Wire je nástroj, který tohle skládání **napíše za tebe** — ale ne za běhu programu, nýbrž jednou dopředu při kompilaci. Ty mu dáš seznam „výrobních funkcí" (konstruktorů), Wire si domyslí, co do čeho patří, a vygeneruje obyčejný Go soubor, který to všechno postaví ve správném pořadí. Výhoda: když něco nepasuje, dozvíš se to hned (chyba při překladu), ne až aplikace spadne uživateli.

## How it works

Dva soubory v `app/infrastructure/di/`:

```
container_provider.go   # ručně psaný „recept" (build tag: //go:build wireinject)
wire_gen.go             # vygenerovaný kód (//go:build !wireinject) — NIKDY needituj ručně
```

**Build tagy je drží odděleně.** Při normálním buildu se kompiluje jen `wire_gen.go` (`!wireinject`); `container_provider.go` se přidá do překladu jen pro samotný nástroj `wire` (`wireinject`). Proto v repu žijí dvě funkce stejného jména `CreateApplication`, ale do binárky se dostane jen ta vygenerovaná.

**`CreateApplication` v `container_provider.go` je jen kostra (stub):** tělo je `wire.Build(...)` se seznamem providerů a pak `return nil, nil`. To `nil, nil` **není bug** — je to kanonický Wire vzor. Wire ten seznam přečte, domyslí závislosti a vygeneruje skutečné tělo `CreateApplication` ve `wire_gen.go`, kde se konstruktory volají ve správném pořadí. Vstupní bod: `cmd/main.go:53` volá `di.CreateApplication(logger, reporter)`.

**Tři druhy záznamů ve `wire.Build(...)`:**

1. **Holé konstruktory** — `sqliteuser.NewRepository`, `security.NewJwtService`, `handler.NewAuthHandler`. Wire si z jejich parametrů sám odvodí, co potřebují.
2. **`provideX` funkce** — adaptér, když constructor vrací konkrétní typ, ale graf potřebuje interface, nebo když je třeba z configu vyrobit typovanou hodnotu:
   ```go
   func providePasswordHasher() shared.PasswordHasher { return security.NewPasswordHasher() }
   func provideCookieSecure(cfg *config.Config) handler.CookieSecure { return handler.CookieSecure(cfg.CookieSecure) }
   ```
3. **`wire.Bind(new(Iface), new(*Concrete))`** — řekne Wire „kdykoli někdo chce `Iface`, dej mu `*Concrete`". Bez toho Wire interface nepropojí.
   ```go
   wire.Bind(new(user.Repository), new(*sqliteuser.Repository))
   wire.Bind(new(token.TokenRepository), new(*sqlitetoken.Repository))
   wire.Bind(new(shared.AuditLogger), new(*sqliteaudit.Repository))
   ```

**Registry-style providery (single source of truth):** některé `provideX` vrací seznam, který je jediným místem registrace dané věci — `providePermissionsRegistry` (seznam command/query s permission), `provideSchedulerJobs` (periodické úlohy), `provideEventHandlers` a `provideRunHandlerRegistry`. Nový záznam přidáš sem, zbytek grafu zůstane beze změny.

> Pozn.: `provideEventHandlers` má dnes jen zakomentovaný příklad a `provideRunHandlerRegistry` vrací **prázdnou mapu** — jsou to zatím **prázdné registry** připravené na budoucí handlery, ne hotová funkcionalita.

## Recipe

### Recipe: přidat novou službu do DI

1. Napiš constructor `func NewX(deps...) *X` (nebo `(*X, error)`) ve své vrstvě/balíčku.
2. Přidej ho do seznamu ve `wire.Build(...)` v `container_provider.go`.
3. Pokud `X` splňuje doménový **interface** (handlery na něm závisí přes interface, ne přes konkrétní typ), přidej řádek `wire.Bind(new(<Iface>), new(*<X>))`.
4. `make di` — přegeneruje `wire_gen.go` (interně `cd app/infrastructure/di && wire`). Tady se hned ukáže, jestli Wire umí graf složit.
5. Ověř buildem: `make dev` (debug binárka).

### Kdy regenerovat (`make di`)

- Vždy po editaci `container_provider.go` (nový provider, nový `wire.Bind`).
- **Po změně signatury libovolného konstruktoru, který je v grafu** — jinak má `wire_gen.go` zastaralé volání.
- `make di` můžeš pustit i samostatně, jen ať chytíš Wire chybu bez plného buildu. `make dev` i `make build` ale mají `di` jako závislost (`dev: di`, `build: di fe-build`), takže **při běžném buildu se přegeneruje samo**.

## Invariants & pitfalls

- **`wire_gen.go` se NIKDY needituje ručně** — je generovaný. Veškeré změny jdou přes `container_provider.go` + `make di`.
- **`return nil, nil` v `container_provider.go` je záměr**, ne nedodělek — Wire stub. Needituj ho.
- **Handlery závisí na doménových interfaces, ne na konkrétních typech** (CLAUDE.md invariant „Domain interfaces only"): command/query handlery, seeder i CLI berou `user.Repository`, nikdy `*sqliteuser.Repository`. `wire.Bind` je přesně to, co tohle umožní zkompilovat — naváže abstraktní `user.Repository` na konkrétní `*sqliteuser.Repository`.
- **Zastaralý `wire_gen.go` = hlasitá chyba, ne tiché špatné zapojení.** Když změníš signaturu konstruktoru a zapomeneš `make di`, build selže na špatném počtu argumentů — selhání je vidět hned a `make dev`/`make build` ho stejně přegenerují za tebe.
- **Nový bounded kontext = víc než DI.** Přidání `domain/order/` a jeho repo znamená i editaci `.go-arch-lint.yml` (viz `/gk-architecture`, `/gk-feature`), nejen řádek ve `wire.Build`.

## Related

- Skills: `/gk-feature` (DI je krok 6 v end-to-end checklistu featury), `/gk-architecture` (kam infrastructure/DI patří ve vrstvách a proč), `/gk-bus` (busy, které se zde wirují přes `provideCommandBus`/`provideQueryBus`/`provideEventBus`), `/gk-config` (`config.LoadConfig` jako provider v grafu)
- Kód: `app/infrastructure/di/container_provider.go` (recept + `provideX` + `wire.Bind`), `app/infrastructure/di/wire_gen.go` (generovaný výstup), `cmd/main.go` (`di.CreateApplication` na ř. 53), `Makefile` (target `di`)
