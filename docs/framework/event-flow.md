---
layout: 'page'
uri: '/framework/event-flow'
position: 65
slug: 'framework-event-flow'
parent: 'framework'
navTitle: 'Event flow'
title: 'Event flow'
description: 'Doménové eventy se sbírají per-request a rozešlou až po commitu transakce; při rollbacku se zahodí.'
---

# Event flow

Doménový event je primitivní fakt „stalo se X" (`user.created`), na který může reagovat kdokoli další, aniž by ho command handler znal. Handler event jen **posbírá** do per-request kolektoru v `ctx`. Teprve po úspěšném **commitu** business transakce `DispatchEventsMiddleware` kolektor vyprázdní a synchronně rozešle handlerům přes `EventBus`. Při rollbacku se eventy zahodí.

> Přehled toku. Návod „vyhlásit event a zaregistrovat handler" → `/gk-domain-events`; busy a middleware → `/gk-bus`.


## K čemu to je

Když chceš na změnu stavu **navázat vedlejší efekt** (uvítací mail, synchronizace, navazující akce), ale nechceš jím zatěžovat command handler. Záruka: **event = potvrzená skutečnost** — rozešle se jen to, co se opravdu zapsalo. Pro pomalou práci nebo práci odolnou vůči pádu procesu event handler nedělá nic sám, ale zařadí run (`RunDispatcher`).


## Jak to teče

1. `DispatchEventsMiddleware` vloží do `ctx` prázdný per-request `*EventCollector`.
2. Command handler posbírá event přes `Collect` (jen primitiva — string ID, čas; žádné entity).
3. Proběhne transakce. Když handler nebo commit **selže** → eventy se zahodí (zmizí spolu s rollbackem).
4. Po úspěšném **commitu** middleware kolektor vyprázdní (`Flush`) a pro každý event volá `eventBus.Dispatch`.
5. `EventBus` spustí registrované handlery **synchronně a sekvenčně**, každý přes `ExecVoid` (Recovery + Logging). Chyba handleru původní command neshodí.

`Dispatch` nejdřív z `ctx` odebere kolektor — kdyby se event handler pokusil zavolat `Collect` na další event, **vyvolá to paniku** (kaskádové eventy nejsou podporované; navazující práce patří do `RunDispatcheru`).


## Příklad

Sběr v command handleru — handler k `EventBus` nepřistupuje přímo:

```go
// app/application/user/command/create_user.go
shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{
    UserID:    u.ID,
    Nickname:  u.Nickname,
    Email:     u.Email,
    Role:      u.Role,
    Timestamp: time.Now(),
})
```

Handlery se registrují na jednom místě — `provideEventHandlers()` v DI (`eb.Register(event, handler)`); volá se jen při sestavování DI.


## Související

- [Command flow](/framework/command-flow) — kde se eventy sbírají a kdy se rozešlou.
- [Architecture](/framework/architecture) — kam event flow zapadá ve vrstvách.
- Skilly: `/gk-domain-events`, `/gk-bus`, `/gk-runs` (navazující asynchronní práce).
