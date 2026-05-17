---
layout: 'page'
uri: '/framework/application/events'
position: 4
slug: 'framework-application-events'
parent: 'framework-application'
navTitle: 'Events'
title: 'Events'
description: 'Domain events -- jak vyhlásit "stalo se X" tak, aby na to mohl reagovat kdokoliv jiný, aniž by o tom command handler musel vědět.'
---

# Events


## K čemu ti to je

Představ si: dopsals `CreateUserHandler` -- vezme nickname a heslo, uloží uživatele do DB, vrátí 201. Spokojený mergneš. Druhý den přijde zadání: po vytvoření uživatele se musí poslat welcome email.

Naivní řešení: do `CreateUserHandler` přihodíš `h.mailer.Send(...)`. Funguje, ale:

- Handler teď ví o mailru, i když jeho práce je "vytvoř uživatele".
- Test handleru potřebuje mock mailru.
- Až přijde další zadání ("a taky pošli notifikaci do Slacku, a založ řádek v audit logu, a..."), bude handler znát všechno na světě.

Lepší řešení: handler **vyhlásí** "stal se UserCreated event" a vůbec ho nezajímá, kdo na to reaguje. Mailer se na ten event zaregistruje samostatně. Kdokoli další (Slack notifier, audit, indexace) se zaregistruje stejně. Handler zůstane jednoduchý.

Tomu se říká **domain events**. Co tím získáš:

1. **Loose coupling.** Command handler ví, co se stalo, ne co se má teď stát.
2. **Atomicita s DB.** Event se rozešle handlerům **až po úspěšném commitu transakce**. Když commit selže, event se zahodí -- žádný welcome email pro uživatele, který v DB nikdy nevznikl.
3. **Izolace mezi requesty.** Dva paralelní `CreateUser` requesty mají vlastní sběrače; eventy se mezi nimi neprolejou.


## Jak to funguje (zjednodušeně)

```
1. HTTP request přijde, command handler začne pracovat
2. Bus otevře DB transakci
3. Handler dělá svou práci (Save user, atd.)
4. Handler vyhlásí event:  EventCollectorFromContext(ctx).Collect(UserCreated{...})
5. Bus commitne transakci
   ├─ commit OK   → bus pošle všechny nasbírané eventy registrovaným handlerům (synchronně)
   └─ commit fail → bus eventy zahodí, vrátí chybu
6. Handler dokončí, HTTP response odejde uživateli
```

Sběrač eventů (`EventCollector`) žije v `ctx` po dobu jednoho requestu. Vytvoří ho middleware `DispatchEvents` na začátku a flushne ho po úspěšném commitu transakce.


## Krok za krokem: přidání nového eventu

Scénář: po vytvoření uživatele chceš poslat welcome email.

### 1. Definuj event v doméně

`domain/user/user_created.go` -- jen primitivy, žádné entity ani VOs (eventy musí být serializovatelné a nezávislé na cizích kontextech):

```go
package user

type UserCreated struct {
    UserID    string
    Nickname  string
    Email     string
    Timestamp time.Time
}

func (e UserCreated) EventName() string     { return "user.created" }
func (e UserCreated) OccurredAt() time.Time { return e.Timestamp }
```

### 2. Vyhlas event v command handleru

`application/user/command/create_user.go` -- collectuj **až po úspěšném zápisu**, ne před, ať se na rollback nedispatchne event pro neexistující data:

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... validace, hash hesla ...
    if err := h.users.Save(ctx, u); err != nil {
        return err
    }

    shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{
        UserID:    u.ID,
        Nickname:  u.Nickname,
        Email:     u.Email,
        Timestamp: time.Now(),
    })
    return nil
}
```

### 3. Napiš handler, který na event zareaguje

`application/user/event/send_welcome_email.go`:

```go
package event

type SendWelcomeEmailHandler struct {
    mailer Mailer
}

func NewSendWelcomeEmailHandler(mailer Mailer) *SendWelcomeEmailHandler {
    return &SendWelcomeEmailHandler{mailer: mailer}
}

func (h *SendWelcomeEmailHandler) Handle(ctx context.Context, event shared.DomainEvent) error {
    e := event.(user.UserCreated)
    return h.mailer.Send(e.Email, "Welcome!", /* tělo */)
}
```

### 4. Zaregistruj handler

`infrastructure/di/container_provider.go` -- jediné místo, kde se říká "tahle aplikace zná tyhle eventy a tohle s nimi dělá". Stejný pattern jako [permissions](/guides/permissions), scheduler jobs, job handlers.

```go
func provideEventHandlers(welcomeMailer *eventcmd.SendWelcomeEmailHandler) []bus.EventHandlerEntry {
    return []bus.EventHandlerEntry{
        {Event: "user.created", Handler: welcomeMailer.Handle},
    }
}
```

A přidáš provider do `wire.Build(...)`. `make di` to zapeče.

Hotovo. Při příštím `POST /api/v1/admin/users` se po commitu zavolá `SendWelcomeEmailHandler.Handle`.


## Co se ti hodí vědět

### Dispatch je synchronní

`EventBus.Dispatch` volá handlery **sériově v request goroutině**. Pomalý handler tedy prodlouží HTTP response. Pro pomalé věci (odeslání emailu přes SMTP, externí API, retry-prone I/O) **nepoužívej event handler** -- patří do perzistentní [job queue](/framework/infrastructure/job-queue). Handler tam jen `JobDispatcherFromContext(ctx).Enqueue(...)` a vrátí se hned.

### Když handler selže

Command už commitnul. Selhání handleru se zaloguje (recovery middleware), ale uživatel dostane normální response (200/201). Z handleru nemůžeš "odvolat" command.

### Eventy přenášejí jen primitivy

Žádné `*User`, žádné value objects. Důvody:

- Cross-domain handler (z jiného bounded contextu) by jinak musel importovat tvou doménu.
- Až přesuneš heavy handler do job queue, payload se serializuje do JSON -- primitivy projdou bez problému.
- Žádný lazy-load hazard při deserializaci v jiném procesu.

### Bez kaskády

Event handler **nesmí** sám volat `EventCollectorFromContext(ctx).Collect(...)`. Sběrač se flushuje jen jednou, takže nové eventy by se tiše ztratily. Když handler potřebuje vyvolat další asynchronní práci, použij `JobDispatcherFromContext(ctx).Enqueue(...)`.

### Mimo bus se eventy tiše zahodí

CLI příkaz `./bin/app create-user` volá `CreateUserHandler.Handle` přímo (bus bypass). V `ctx` žádný sběrač není; `EventCollectorFromContext` vrátí *throwaway* instanci. `Collect` projde, ale eventy nikam nejdou. Pro one-shot CLI (vytvořit admina při deployi) je to žádoucí default -- žádný welcome mail pro seedovaného admina.

### Goroutiny v handleru

Pokud handler spustí vlastní goroutiny a všechny volají `Collect`, je to bezpečné -- `EventCollector` má `sync.Mutex`. Není to běžný pattern, ale když to potřebuješ, není potřeba nic ošetřovat.


## Co lze nastavit

Vše živé v `infrastructure/di/container_provider.go`, žádné env proměnné.

| Co | Kde | Default | Jak změnit |
|---|---|---|---|
| Které eventy aplikace zná | `provideEventHandlers()` | prázdný slice | Přidej `EventHandlerEntry` |
| Více handlerů na stejný event | Více entries se stejným `Event` | jeden handler per event | Přidej další entry; volají se v pořadí registrace |
| Middleware kolem handleru | `provideEventBus()` | `Recovery + Logging` | Přidej / vyměň middleware ve volání `NewEventBus(...)` |
| Atomicita s commitem | Pořadí middleware v `provideCommandBus` | `DispatchEvents` wrapuje `Transaction` | Neměň -- záměrný invariant (změna otevře okno "event poslán, commit selhal") |
| Sync → async dispatch | n/a | sync v request goroutině | Není konfigurovatelné. Pro async přesuň handler do [job queue](/framework/infrastructure/job-queue) -- `Enqueue` z event handleru je sub-milisekundový zápis do DB. |
