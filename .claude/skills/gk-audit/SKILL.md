---
layout: 'page'
uri: '/skills/gk-audit'
position: 30
slug: 'skills-gk-audit'
parent: 'skills-observability'
navTitle: 'gk-audit'
title: 'GK — Audit log'
description: 'Append-only záznam security-relevantních akcí, který přežije rollback business transakce — kdo, kdy, odkud a co udělal. Use when přidáváš audit zápis (login, smazání uživatele, theft detection), ladíš „proč mi failed login nezapadl do auditu", nebo řešíš, kde v middleware chainu audit leží.'
name: 'gk-audit'
---

# GK — Audit log

Každá security-relevantní akce se zapíše jako jedna řádka do tabulky `audit_log`.
Zápis přežije i rollback business transakce — failed login se uloží, i když se
celá operace zruší.

## What & when
- Sáhni sem, když chceš zaznamenat **kdo / kdy / odkud / co** u akce, která se
  týká bezpečnosti: login (úspěch i selhání), lock účtu, theft detection,
  vytvoření / smazání uživatele, změna role, změna hesla.
- Sáhni sem, když řešíš „proč můj `Record` zápis nezůstal po rollbacku" nebo
  „kam v middleware chainu audit patří".
- **Netýká se** běžného strukturovaného logování (to je `/gk-logging`),
  domain eventů pro vedlejší efekty (`/gk-domain-events`), ani lockout / rate
  limit politiky (`/gk-rate-limiting`).

## For non-tech / juniors
Audit log je **kniha návštěv pro bezpečnost**: na rozdíl od běžných logů, kde se
často skartuje a přepisuje, se sem jen **dopisuje** (append-only) a nikdy se to
nemaže. Když pak přijde stížnost „někdo mi smazal admin účet", otevřeš tuhle
knihu a vidíš jednu jasnou řádku: kdo (`actor_user_id`), z jaké IP (`actor_ip`),
kdy (`created_at`) a co (`action`) — bez lovení v tisících řádků aplikačního logu.

Klíčový trik: i kdyby se operace nakonec **zrušila** (rollback — databáze se
vrátí do původního stavu, jako by se nic nestalo), audit řádka tam **zůstane**.
Přesně proto, že failed login je z definice akce, která „neproběhla úspěšně" —
ale ty o ní chceš vědět.

## How it works
Tok zápisu má tři aktéry: tvůj handler (co se stalo), middleware (kdo/kdy/odkud)
a repozitář (uložení mimo transakci).

**1. Handler zaznamená událost** — `app/domain/shared/audit.go` (`AuditEvent`):
```go
shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
    Action:     "user.created",            // dotted <domain>.<event>
    TargetType: "user",
    TargetID:   u.ID,
    Metadata:   map[string]any{"role": u.Role},  // libovolný JSON-able map
})
```
Plníš jen `Action`, `TargetType`, `TargetID`, `Metadata`. Zbytek doplní middleware.

**2. `AuditMiddleware` obohatí a uloží** — `app/application/bus/middleware/audit.go`:
- Vytvoří **per-request** `AuditCollector` a strčí ho do `ctx` (žádný singleton —
  paralelní commandy se nemíchají).
- Po handleru (i když vrátil error) `Flush()`-ne nasbírané eventy.
- Pro každý doplní `actor_user_id` (z `ClaimsFromContext`), `actor_ip`
  (z `ActorIPFromContext`, plněno HTTP middleware v `presentation/http/middleware/ip.go`),
  `created_at` a `id` (UUID) → `AuditRecord` → `AuditLogger.Save`.
- Flush běží na `context.WithoutCancel(ctx)` — odpojený klient nesmí zabít zápis.

**3. Repozitář píše mimo transakci** — `app/infrastructure/sqlite/audit/repository.go`
volá `r.DB.DB()` (raw connection pool), **ne** `r.Conn(ctx)`. Proto INSERT
necommituje spolu s business transakcí a přežije její rollback. Schema:
`migrations/20260327000001_init_schema.sql`.

**Pozice v chainu (klíčová)** — audit leží **vně** transakce:
```
Recovery → Logging → Authorize → Tenant → Audit → RunDispatcher → DispatchEvents → Transaction → handler
                                          ^^^^^                                     ^^^^^^^^^^^
                                          audit zde, tx vně → rollback audit nesmaže
```
Pořadí je jednozdrojové v `middleware.CommandChain` (`app/application/bus/middleware/base.go`) — DI provider (`provideCommandBus` v `container_provider.go`) i testfx ho jen volají, takže se nemohou rozjet.

**Mimo bus** — bez instalovaného collectoru (přímé volání handleru v testu) vrátí
`AuditCollectorFromContext` jednorázový collector a `Record` se neperzistuje. Handler
tedy nikdy nil-checkuje. CLI commandy (create-*, seed) jedou přes `SystemCommandBus`,
který AuditMiddleware má, a background run handlery drainuje run worker do
`AuditLogger` sám (`app/infrastructure/worker/run_worker_audit.go`) — v obou
případech se audit záznamy perzistují.

## Recipe
Přidat audit zápis do command handleru:
1. Po **úspěšném** zápisu (nebo v daném failure větvi) zavolej
   `shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{...})`.
2. `Action` = dotted lowercase `<domain>.<event>` (`auth.login.failed`,
   `user.role_changed`). Drž konvenci — pomáhá grepu.
3. Citlivé detaily dej do `Metadata` (např. `{"nickname": nickname}`).
   `actor_user_id` / `actor_ip` / čas **nestav ručně** — doplní je middleware.
4. Žádná DI ani route změna — `AuditMiddleware` už v chainu je. Hotovo.

## Invariants & pitfalls
- **Audit MUSÍ zůstat vně transakce.** Když přesouváš middleware, nech audit
  před `TransactionMiddleware` — jinak rollback smaže i security trail.
- **Raw pool je úmyslný.** `audit/repository.go` používá `r.DB.DB()` schválně
  (viz `/gk-repositories`). Stejná výjimka jako `RecordFailedLogin`. Žádný jiný
  repozitář to nedělá.
- **Failure-safe.** Pád `Save` se zaloguje (`action`, `command`, `error`), ale
  **nikdy** se nepropaguje volajícímu — degradovaný trail je lepší než 500.
- **Append-only je dané kódem, ne DB constraintem.** Repozitář vystavuje jen
  `Save` (INSERT) — žádný Update/Delete. Není to vynuceno triggerem; drž to při
  psaní dalšího kódu.
- **Eventy nesou primitivy.** `Metadata` = JSON-serializovatelné hodnoty
  (string ID, čas), ne entity ani value objekty.
- **Konvence `Action`.** Reálné akce dnes v kódu, pro inspiraci formátu:
  `auth.login.succeeded` / `auth.login.failed` (`{nickname}`) /
  `auth.login.blocked_while_locked` (správné heslo na zamčený účet) /
  `auth.account.locked` (`{locked_until}`) / `auth.token.theft_detected`
  (`{reason: reused_after_rotation | concurrent_rotation_race}`) /
  `auth.logout` (global session revocation) /
  `user.created` (`{role}`) / `user.role_changed` (`{new_role}`) /
  `user.deleted` / `user.password_changed` / `tenant.created`.

## Related
- Skills: `/gk-bus` (middleware chain + pořadí), `/gk-auth` (kde většina audit
  eventů vzniká), `/gk-rate-limiting` (lockout / login-lock politika a klientská
  IP), `/gk-repositories` (raw-pool výjimka), `/gk-domain-events` (stejný
  per-request collector vzor)
- Kód: `app/domain/shared/audit.go`, `app/application/bus/middleware/audit.go`,
  `app/infrastructure/sqlite/audit/repository.go`,
  `migrations/20260327000001_init_schema.sql`,
  `app/infrastructure/di/container_provider.go` (`wire.Bind` AuditLogger),
  `app/presentation/http/middleware/ip.go` (actor IP)
