---
layout: 'page'
uri: '/framework/infrastructure/scheduler'
position: 5
slug: 'framework-infrastructure-scheduler'
parent: 'framework-infrastructure'
navTitle: 'Scheduler'
title: 'Scheduler'
description: 'In-process scheduler -- jak nastavit, že se něco má provádět pravidelně (každou hodinu, každý den) uvnitř běžícího serveru.'
---

# Scheduler


## K čemu ti to je

Občas potřebuješ něco dělat pravidelně sám od sebe -- typicky **maintenance**: smazat expirované tokeny, warmnout cache, emitovat metriku. Tradičně by to dělal externí `cron`. Gokick to umí uvnitř -- žádný druhý systém, jeden binary.

Tři důvody, proč to dělat takhle:

1. **Single binary deploy.** Když nasadíš `serve`, scheduler běží s ním.
2. **Sdílí lifecycle se serverem.** Jeden SIGTERM ukončí obojí, žádné osamělé goroutiny.
3. **První tick hned po startu**, ne až za hodinu. Frekventně restartovaný proces (deploys, dev) stále garantuje aspoň jeden tick za lifetime.

Co tam **nepatří**: práce, která musí přežít restart procesu (welcome maily, externí API). To je [Job Queue](/framework/infrastructure/job-queue).


## Krok za krokem

Scénář: každou hodinu smazat expirované refresh tokeny.

### 1. Funkce, kterou chceš pravidelně volat

Signatura musí být `func(ctx context.Context) error`. Pro náš případ už existuje v `token.TokenRepository.DeleteExpired`.

### 2. Zaregistruj job v `provideSchedulerJobs`

`infrastructure/di/container_provider.go` -- jediné místo, stejný pattern jako [events](/framework/application/events) nebo [permissions](/guides/permissions):

```go
func provideSchedulerJobs(tokens token.TokenRepository) []scheduler.Job {
    return []scheduler.Job{
        {
            Name:     "cleanup:expired-refresh-tokens",
            Interval: 1 * time.Hour,
            Fn:       tokens.DeleteExpired,
        },
    }
}
```

### 3. `make di` a hotovo

Při příštím `make serve` uvidíš v logu:

```
{"msg":"scheduler: starting","jobs":1}
{"msg":"scheduler: job completed","name":"cleanup:expired-refresh-tokens","duration":"333µs"}
```

Druhý řádek je **run-once tick** -- proběhl ihned, ne až za hodinu.


## Co se ti hodí vědět

- **Run-once-then-tick**: každý job se spustí hned po startu, pak ve zvoleném intervalu. Garantuje aspoň jeden tick za životnost procesu.
- **Panic v jobu se zachytí**, zaloguje, další tick proběhne normálně. Sourozenecké joby běží dál.
- **Error z `Fn` se loguje, ticker tiká dál** -- předpokládá se idempotence.
- **Validace při startu**: duplicitní jméno, nulový interval, nil `Fn` -- proces ani nestartuje (fail-fast).
- **HTTP server běží paralelně.** DB volání jsou OK (Wire je hotový), ale **nevolej z jobu vlastní HTTP API přes localhost** -- proběhne to jako závod.
- **Multi-instance**: scheduler je in-process, dvě repliky tickají nezávisle. Pro single-execution napříč clusterem přidej DB lock do `Fn`.


## Co lze nastavit

| Co | Kde | Default |
|---|---|---|
| Které joby aplikace spouští | `provideSchedulerJobs()` v `container_provider.go` | jen `cleanup:expired-refresh-tokens` |
| Interval konkrétního jobu | Pole `Interval` v `scheduler.Job` | nastavený v provideru |
| Multi-instance koordinace | n/a | žádná -- pro single-execution napříč replikami DB lock přímo v `Fn` |
