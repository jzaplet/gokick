---
layout: 'page'
uri: '/framework/application/audit'
position: 5
slug: 'framework-application-audit'
parent: 'framework-application'
navTitle: 'Audit log'
title: 'Audit log'
description: 'Append-only zápis security-relevantních akcí, který přežije rollback business transakce.'
---

# Audit log


## K čemu ti to je

Když přijde stížnost _"někdo mi smazal admin účet"_, chceš najít kdo, kdy a odkud — bez toho, abys to musel rekonstruovat z application logu. Když ti backend hlásí 5 failed loginů za minutu, chceš vědět, na jaký nickname jely a z jaké IP. Když útočník zneužije refresh token, chceš mít čistý audit trail, ne 500 řádků slogu.

Audit log dělá jednu věc: zapíše každou security-relevantní akci jako jednu řádku do tabulky `audit_log`. Append-only — nikdo (včetně aplikace) řádky nemění.

Tři garance:

1. **Survive business rollback.** Když handler vrátí `AuthError` a celá business transakce se zruší, audit zápis tam zůstane. Login_failed musí persistovat — to je celý pointa.
2. **Per-request izolace.** Stejně jako [eventy](/framework/application/events), každý request má vlastní collector. Žádný leak mezi paralelními commandy.
3. **Failure-safe.** Pád audit zápisu se loguje, ale nikdy nezhasí business operaci. Degradovaný trail je lepší než 500.


## Krok za krokem

Scénář: po `CreateUser` zaznamenat, kdo a koho založil.

### 1. Zavolej `Record` v handleru po úspěšném zápisu

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... business validation + save ...

    if err := h.users.Save(ctx, u); err != nil {
        return err
    }

    shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
        Action:     "user.created",
        TargetType: "user",
        TargetID:   u.ID,
        Metadata:   map[string]any{"role": u.Role},
    })

    return nil
}
```

`Action` je dotted string `domain.event` (např. `auth.login.failed`, `user.role_changed`). `Metadata` je libovolný JSON-serializovatelný map.

### 2. Co se stane dál

`AuditMiddleware` v command bus chainu:

1. Vytvoří collector a strčí ho do `ctx` před voláním handleru.
2. Handler runs, possibly does `Record(...)`.
3. Po handleru (success nebo error) middleware drainuje collector.
4. Pro každý event vytvoří `AuditRecord` (přidá `actor_user_id` z `ClaimsFromContext`, `actor_ip` z `ActorIPFromContext`, timestamp) a pošle ho do `AuditLogger.Save(...)`.
5. Save runs na raw connection pool — **mimo** business transakci, takže přežije rollback.

### 3. Co když je handler volaný mimo bus

`AuditCollectorFromContext` vrací **throwaway collector**, když ho nikdo do `ctx` neinjectoval (CLI, testy). Record je no-op — handler tedy nemusí nil-checkovat ani v testech, ani v `./bin/app create-user` bypassu.

### 4. Failure-safe contract

Pokud `AuditLogger.Save` selže (např. disk full):

- Chyba se zaloguje s `action`, `command`, `error`.
- Handler ji **nevidí** — business response zůstává úspěšná.

Důvod: audit trail je best-effort. Degradace logging > shoz produkce.


## Co se ti hodí vědět

**Architecture rule** — `AuditMiddleware` MUSÍ ležet outside `TransactionMiddleware` a `DispatchEventsMiddleware`. Pokud ji někdy přesouváš, drž tohle:

```
Recovery → Logging → Authorize → Audit → JobDispatcher → DispatchEvents → Transaction → handler
                              ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                              Audit zde, tx vně, takže rollback audit nesmaže.
```

**`r.DB.DB()` výjimka** — `infrastructure/sqlite/audit/repository.go` používá raw connection pool, ne `r.Conn(ctx)`. To je porušení obecného pravidla, ale **úmyslné**: audit nesmí joinovat caller's tx. Stejně tak `user.Repository.RecordFailedLogin` / `ResetFailedLogin` (brute-force counter). Jiné repos to nedělají.

**Detached ctx pro flush** — middleware používá `context.WithoutCancel(ctx)` aby disconnected klient nemohl zabít audit zápis prostředky cancel signal.

**Co loguješ ty, co middleware** — `Action`, `TargetType`, `TargetID`, `Metadata` jsou tvé. `actor_user_id`, `actor_ip`, `created_at`, `id` (UUID) doplní middleware. Nikdy je nestavej do `Metadata` ručně.

**Konvence pro `Action`** — dotted lowercase `<domain>.<event>` (`auth.login.succeeded`, `user.password_changed`). Pomáhá to grep + budoucímu observability nástroji.


## Co lze nastavit

Žádné env vary. Lock policy je v `application/auth/command/login.go` jako konstanty (`loginLockThreshold`, `loginLockWindow`, `loginLockDuration`). Audit nemá žádný runtime config — buď je nasazený a píše, nebo není.

| Konstanta | Hodnota | Význam |
|---|---|---|
| `loginLockThreshold` | `5` | Failed loginů uvnitř `loginLockWindow` → lock. |
| `loginLockWindow` | `10m` | Reset counter, pokud poslední fail byl dál v minulosti. |
| `loginLockDuration` | `15m` | Délka locku po dosažení threshold. |


## Eventy, které aplikace dnes loguje

| Action | Kde se vyhlašuje | Metadata |
|---|---|---|
| `auth.login.succeeded` | `LoginHandler` po vydání tokenu | — |
| `auth.login.failed` | `LoginHandler` po failed Verify | `{nickname}` |
| `auth.login.blocked_while_locked` | `LoginHandler` když attempt na locked účet | — |
| `auth.account.locked` | `LoginHandler` po dosažení threshold | `{locked_until}` |
| `auth.token.theft_detected` | `RefreshTokenHandler` (used_at != nil OR concurrent race) | `{reason}` |
| `user.created` | `CreateUserHandler` | `{role}` |
| `user.role_changed` | `UpdateUserHandler` jen pokud role změnila | `{new_role}` |
| `user.deleted` | `DeleteUserHandler` | — |
| `user.password_changed` | `ChangePasswordHandler` | — |
