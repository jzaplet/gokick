---
layout: 'page'
uri: '/skills/gk-commands'
position: 20
slug: 'skills-gk-commands'
parent: 'skills-cqrs'
navTitle: 'gk-commands'
title: 'GK — Command handlery (write operace)'
description: 'Psaní command handlerů (write operace) — struktura command + handler, povinná deklarace permission (Permissioned / SkipPermission) a validace přes value objects. Use when přidáváš nebo upravuješ write operaci (vytvoř/uprav/smaž entitu, login, logout) a potřebuješ vědět, jak má handler vypadat a co musí deklarovat.'
name: 'gk-commands'
---

# GK — Command handlery (write operace)

Command = jedna write operace, která mění stav systému. Píše se jako dvojice: `XxxCommand` (jen data) + `XxxHandler` (logika).

## What & when
- Sáhni sem, když píšeš operaci, která něco **zapisuje / mění** — `CreateUser`, `UpdateUser`, `DeleteUser`, `Login`, `Logout`, `RefreshToken`.
- Pro **čtení** (read-only, vrací data, nic nemění) se píše query, ne command — jiný vzor, jiný bus (`/gk-queries`).
- Tohle NEpopisuje, kudy command teče busem a jaké middleware se spustí — to řeší `/gk-bus`. Tady jde čistě o to, jak handler napsat.

## For non-tech / juniors
Představ si command jako **objednávkový lístek**: `XxxCommand` je vyplněný lístek se vstupy (jméno, heslo, role) — žádná logika, jen data. `XxxHandler` je kuchař, který ten lístek vezme a vykoná práci (zvaliduje, zapíše do DB, vyhlásí „stalo se").

Klíčové pravidlo: každý lístek musí mít napsáno, **kdo ho smí podat** (jakou permission to vyžaduje), nebo výslovně „tohle je veřejné". Když to chybí, systém objednávku odmítne — to je pojistka proti zapomenuté kontrole oprávnění.

## How it works
Každý command žije v souboru `app/application/<context>/command/xxx.go` a obsahuje **dva typy**:

- **`XxxCommand`** — data struct, raw hodnoty z HTTP requestu (`string`, …). Žádná logika.
- **`XxxHandler`** — logika. Konstruktor `NewXxxHandler(...)` bere **jen doménové interfaces** (`user.Repository`, `shared.PasswordHasher`, `shared.TokenService`), nikdy konkrétní infrastrukturu. Metoda `Handle(ctx, cmd)` dělá práci.

**Deklarace permission je povinná** (`app/domain/shared/permission.go`). Command implementuje právě jedno:
- `RequiredPermission() string` (interface `Permissioned`) — např. `create_user.go:18` vrací `"admin:users:create"`.
- `SkipPermissionCheck()` (interface `SkipPermission`) — výslovný opt-out pro veřejné operace, např. `login.go:17` / `refresh_token.go:16`.

Když command neimplementuje ani jedno, `AuthorizeMiddleware` vrátí runtime error `bus: command %q must implement Permissioned or SkipPermission` (`app/application/bus/middleware/authorize.go:20`). Tj. zapomenutí se pozná hned, ne až v produkci.

**Validace uvnitř `Handle`** má dvě úrovně (viz `create_user.go`):
1. **Formát** přes doménové value objects — `user.NewNickname`, `user.NewEmail`, `user.NewRole`, `user.NewPassword`. Vrátí `*shared.ValidationError` na nevalidní vstup. (Modelování value objects: `/gk-entities`.)
2. **Business pravidla (I/O)** přes repo — unikátnost nicku (`repo.FindByNickname` ve sdíleném `userwrite.Create`, `app/application/userwrite/userwrite.go`), existence (`h.users.FindByID` v `update_user.go`).

**Vedlejší efekty** se nesbírají přímo, ale přes per-request collectory z ctx:
- `shared.AuditCollectorFromContext(ctx).Record(...)` — pro security-relevant mutace (`user.created` a `user.role_changed` ve sdíleném `application/userwrite/userwrite.go`, `user.deleted` v `delete_user.go:55`).
- `shared.EventCollectorFromContext(ctx).Collect(...)` — doménový event (`user.UserCreated` v `userwrite.go:169` — sdílené tělo create, viz F-031), na který reaguje někdo jiný. Detail: `/gk-domain-events`.

**Dispatch z HTTP handleru** (detail v `/gk-bus`):
- bez návratové hodnoty → `bus.DispatchVoid(...)`.
- s typovaným výsledkem → `bus.Dispatch[T](...)`, např. `Login` vrací `IssuedSession` (`handler/auth.go:96`). Obě berou `*bus.CommandBus`, takže command nejde omylem poslat přes query bus.

Transakci, autorizaci, audit i rozeslání eventů řídí middleware v busu — handler o nich neví.

## Recipe

<!-- gkdoc:ignore app/application/user/command/archive_user.go — vymyšlený command, který si recept zakládá -->

Přidání nového command handleru (např. `ArchiveUser`):

1. Vytvoř `app/application/user/command/archive_user.go`.
2. Napiš `ArchiveUserCommand` — jen data (`ID string`, …).
3. Přidej deklaraci permission: `func (ArchiveUserCommand) RequiredPermission() string { return "admin:users:archive" }` (nebo `SkipPermissionCheck()` u veřejné operace).
4. Napiš `ArchiveUserHandler` + `NewArchiveUserHandler(users user.Repository)` — **jen doménové interfaces** v konstruktoru.
5. V `Handle(ctx, cmd)`: validuj vstup (value objects) → ověř business pravidla (repo dotazy) → zapiš (`r` přes repo).
6. Pokud jde o security-relevant změnu, zaznamenej `shared.AuditCollectorFromContext(ctx).Record(...)`; pokud má někdo reagovat, `...EventCollectorFromContext(ctx).Collect(...)`.
7. Zaregistruj v DI + napiš HTTP handler, který pošle command přes `bus.DispatchVoid` / `bus.Dispatch` (celý end-to-end checklist: `/gk-feature`).

## Invariants & pitfalls
- **Každý command MUSÍ deklarovat permission** — `Permissioned` nebo `SkipPermission`, jinak runtime error z `AuthorizeMiddleware`. Žádná tichá výjimka.
- **Handler závisí jen na doménových interfaces.** Nikdy neimportuj `infrastructure/security/`, `infrastructure/sqlite/` ani `application/bus` z handleru — Wire injektuje interface, ne konkrétní typ.
- **Command struct nemá logiku** — jen pole. Validace a I/O patří do `Handle`.
- **Většina commandů běží v bus transakci automaticky — neopt-outuj jen tak.** `SkipTransaction()` (interface `shared.SkipsTransaction`, `app/domain/shared/permission.go`; vynucuje ho `bus/middleware/transaction.go`) je vzácná výjimka; reálně ji mají jen `login.go` a `refresh_token.go` kvůli raw-pool zápisům / force-logoutu. Bez konkrétního důvodu ji nepřidávej.
- **Audit/event collectory jsou vždy bezpečné zavolat** — mimo bus (přímé volání handleru v testech) vrátí helper „zahazovací" collector, takže `Handle` nemusí nil-checkovat. CLI commandy jedou přes `SystemCommandBus`, kde se collectory drainnou.
- **`*shared.ValidationError` s `Field` → konkrétní pole na FE.** Vracej ho z validace, ať se chyba namapuje na správný input. Mapování chyb na HTTP status řeší `/gk-errors`.

## Related
- Sousední skills: `/gk-queries` (read operace — protějšek commandu), `/gk-bus` (dispatch + middleware chain), `/gk-feature` (end-to-end checklist nové featury), `/gk-domain-events` (collect & reaguj), `/gk-entities` (value objects & validace), `/gk-errors` (mapování chyb na HTTP), `/gk-architecture` (vrstvy a pravidla závislostí)
- Kód: `app/application/user/command/` (`create_user.go`, `update_user.go`, `delete_user.go`), `app/application/auth/command/` (`login.go`, `logout.go`, `refresh_token.go`), `app/domain/shared/permission.go`, `app/application/bus/middleware/authorize.go`
