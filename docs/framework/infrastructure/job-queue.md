---
layout: 'page'
uri: '/framework/infrastructure/job-queue'
position: 6
slug: 'framework-infrastructure-job-queue'
parent: 'framework-infrastructure'
navTitle: 'Job Queue'
title: 'Job Queue'
description: 'Perzistentní fronta pro background práci -- jak rozjet úlohu, která musí proběhnout i když proces mezitím spadne.'
---

# Job Queue


## K čemu ti to je

[Events](/framework/application/events) řeší synchronní reakci v request goroutině -- skvělé pro rychlé side-effects. Pomalý handler ale prodlouží HTTP response a SIGTERM mu uřízne inflight práci. Pro odeslání emailu přes SMTP, volání externího API nebo cokoli retry-prone potřebuješ **perzistenci**.

Job Queue je SQLite tabulka `jobs`. Command handler zavolá `Enqueue("welcome:send", payload)` -- zapíše se řádek **ve stejné transakci** jako business write. Worker (goroutina nebo samostatný `./bin/app worker` proces) si ho vyzvedne a zavolá handler. Když spadne, retry s exponential backoff. Když crashne celý proces, restart pokračuje tam, kde skončil.

Tři garance:

1. **Atomicita business write + enqueue.** Uložení uživatele a enqueue welcome jobu jdou v jedné DB transakci. Buď obojí, nebo nic.
2. **At-least-once delivery.** Job proběhne minimálně jednou. Handler musí být idempotentní pro **externí** side effects (poslat dva maily je špatně).
3. **Mark-complete v handlerově tx.** "Job hotový" se zapíše ve stejné transakci jako handler's DB writes. Handler-fail = celá tx rollback, žádné částečné stavy.


## Krok za krokem

Scénář: po `CreateUser` poslat welcome email přes SMTP (pomalé, může selhat).

### 1. Handler funkce

`application/user/job/send_welcome.go`:

```go
type WelcomePayload struct {
    UserID string `json:"user_id"`
    Email  string `json:"email"`
}

func (h *SendWelcomeHandler) Handle(ctx context.Context, payload []byte) error {
    var p WelcomePayload
    if err := json.Unmarshal(payload, &p); err != nil {
        return err
    }
    return h.mailer.Send(p.Email, "Welcome!", /* ... */)
}
```

Vrátí error → worker zařadí retry. Vrátí nil → job complete.

### 2. Zaregistruj v `provideJobHandlerRegistry`

`infrastructure/di/container_provider.go` -- stejný pattern jako [events](/framework/application/events) a [scheduler](/framework/infrastructure/scheduler):

```go
func provideJobHandlerRegistry(welcome *jobcmd.SendWelcomeHandler) (*jobapp.HandlerRegistry, error) {
    return jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{
        "welcome:send": welcome.Handle,
    })
}
```

### 3. Enqueue z command handleru

`Enqueue` má povinné `maxRetries` jako poziční parametr -- žádný magický default. `0` = "vykonej jednou, žádný retry" (welcome mail, audit log). Vyšší pro flaky externí volání (`3` = 3 retries po prvním selhání = až 4 attempts).

```go
if err := h.users.Save(ctx, u); err != nil {
    return err
}
return shared.JobDispatcherFromContext(ctx).Enqueue(ctx, "welcome:send", 0, WelcomePayload{
    UserID: u.ID, Email: u.Email,
})
```

Dispatcher zkontroluje, že kind je v registru (chytíš překlep v testu, ne v produkci), payload zapíše do `jobs` -- ve stejné transakci jako `users.Save` výše.

### 4. `make di`

Hotovo. SMTP nefunguje? Když máš `maxRetries=3`, worker to za 5s zkusí znovu, pak 10, 20 ... a po 4. selhání (1 původní + 3 retries) označí `failed` (řádek zůstane pro debug). Pokud máš `maxRetries=0`, po prvním selhání rovnou `failed` -- žádný retry.


## Co se ti hodí vědět

- **Mark-complete v handlerově tx.** Bez toho by crash mezi handler-commit a mark-complete způsobil duplicate side effect. Náš pattern: handler authors přemýšlejí o idempotenci jen pro **externí** side effects -- DB writes se rollbacknou společně s "job hotový" flagem.
- **Default 1 worker.** SQLite serializuje writery (WAL: jeden writer na celou DB). Víc workerů nezvýší throughput DB-bound handlerů. Bumpnout má smysl, jen když jsou handlery I/O-bound mimo SQLite.
- **Standalone `./bin/app worker` proces** spustí jen worker bez HTTP serveru -- vhodné pro split deploy (1× serve + N× worker, sdílená SQLite).
- **Cascade jobs OK, cascade events ne (strojově vynuceno).** Job handler může enqueueovat další jobs (`JobDispatcher` je v ctx). Když ale zavolá `EventCollectorFromContext.Collect(...)`, runtime panic — sběrač eventů se flushuje jen v command request goroutině, ne ve workeru. Worker chybu zachytí, zaloguje, job se reschedule.


## Co lze nastavit

| Co | Kde | Default | Jak změnit |
|---|---|---|---|
| Které kindy worker zná | `provideJobHandlerRegistry()` v `container_provider.go` | prázdná mapa | Přidej `kind → handler.Handle` entry |
| `maxRetries` | Povinný poziční parametr `Enqueue` | bez defaultu (musíš zvolit) | `disp.Enqueue(ctx, "welcome:send", 0, payload)` -- `0` = no retry, `3` = až 3 retries po prvním selhání |
| Odložené spuštění | `shared.WithDelay(d)` při `Enqueue` | spustit ihned | `disp.Enqueue(ctx, kind, 0, payload, shared.WithDelay(time.Hour))` |
| Worker concurrency | `provideWorker` v `container_provider.go` | `1` | Zvyš parametr (pozor na SQLite serializaci) |
| Poll interval / backoff / lock timeout | Konstanty ve `infrastructure/worker/worker.go` | `1s` / `5s base, 1h cap` / `5min` | Pro reálné nasazení vytáhni do configu |
