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

Občas potřebuješ něco dělat **pravidelně, sám od sebe** -- bez HTTP requestu, který by to spustil.

Klasický příklad: v tabulce `refresh_tokens` se hromadí expirované záznamy. Nikdo si jich nikdy nevšimne -- jen tam pomalu rostou, dokud disk nedojde. Někdo to musí jednou za čas smazat.

Nebo: jednou za den chceš zaslat report admin emailu, jednou za hodinu warmnout cache, jednou za 5 minut emitovat metriku do Grafany. Žádná z těch věcí nepatří k HTTP requestu -- jsou to **maintenance úlohy**, které žijí uvnitř procesu.

Tradiční řešení mimo Go aplikaci by bylo `cron` na hostiteli. Gokick to dělá uvnitř -- nepotřebuješ druhý systém, nepotřebuješ separátní binárku, prostě přihodíš job a běží.

Tři důvody, proč to dělat takto:

1. **Single binary deploy.** Žádný externí cron, žádný separátní worker container. Když nasadíš `serve`, scheduler běží s ním.
2. **Sdílí lifecycle se serverem.** Jeden SIGTERM ukončí scheduler i HTTP server zároveň. Žádná osamělá goroutina po restartu.
3. **První tick hned po startu** (ne až za hodinu). Když restartuješ proces, cleanup proběhne hned -- frekventně restartovaný proces (deploys, dev) stále dostane minimálně jeden tick za lifetime.

Co tam **nepatří**: práce, která musí přežít restart procesu nebo crash (welcome emaily, externí API volání, retry-prone I/O). To je dvojnásob jiná disciplína -- [Job Queue](/framework/infrastructure/job-queue).


## Jak to funguje (zjednodušeně)

```
1. main() spustí Application.Run(ctx)
2. ServeCommand spustí scheduler.Run(ctx) v goroutině + server.Start(ctx)
3. Scheduler pro každý registrovaný job spustí vlastní goroutinu:
   ├─ tick #1: zavolá Fn ihned (run-once-then-tick)
   ├─ tick #2: za Interval (např. 1 hodina)
   ├─ tick #3: za další Interval
   └─ ...
4. SIGTERM přijde
   ├─ ctx.Done() se odpálí
   ├─ scheduler v každé goroutině preempne ticker, vrátí se
   ├─ server.Start drainuje inflight requesty
   └─ proces exitne čistě
```

Job je trojice `{Name, Interval, Fn}` -- jméno (pro logy), interval mezi ticky, funkce co se má zavolat. Nic složitějšího.


## Krok za krokem: přidání nového jobu

Scénář: chceš každou hodinu smazat expirované refresh tokeny.

### 1. Napiš funkci, která to udělá

V `domain/token/repository.go` už metoda existuje:

```go
type TokenRepository interface {
    // ...
    DeleteExpired(ctx context.Context) error
}
```

Pokud takovou funkci nemáš, implementuj ji v repository (nebo kdekoli, kde má smysl). Klíčová podmínka: signatura **musí být `func(ctx context.Context) error`**.

### 2. Zaregistruj job v `provideSchedulerJobs`

Jediné místo, kde se přidávají scheduler joby, je `provideSchedulerJobs()` v `infrastructure/di/container_provider.go`. Stejný pattern jako [permissions](/guides/permissions) nebo event handlers.

```go
func provideSchedulerJobs(tokens token.TokenRepository) []scheduler.Job {
    return []scheduler.Job{
        {
            Name:     "cleanup:expired-refresh-tokens",
            Interval: 1 * time.Hour,
            Fn:       tokens.DeleteExpired,
        },
        // přidej další joby sem
    }
}
```

Pokud tvůj job potřebuje další závislost (jiné repository, mailer, atd.), přidej ji jako parametr `provideSchedulerJobs`. Wire ji injektne automaticky.

### 3. `make di`

Wire přegeneruje DI. Žádné další kroky -- scheduler ho při startu uvidí.

### 4. Hotovo

Při příštím `make serve` uvidíš v logu:

```
{"msg":"scheduler: starting","jobs":1}
{"msg":"scheduler: job completed","name":"cleanup:expired-refresh-tokens","duration":"333µs"}
```

První řádek je startup. Druhý je **run-once tick** -- proběhl hned po startu, ne až za hodinu.


## Co se ti hodí vědět

### Run-once-then-tick

Job se spustí **hned** po startu scheduleru, **pak teprve** ve zvoleném intervalu. Důvod: maintenance úloha (typický scheduler usecase) má dávat smysl mít ji garantovanou aspoň jednou za životnost procesu. Když restartuješ proces každých 10 minut (deploys), s ticker-only by se 1h cleanup nikdy neprovedl.

### Když job spadne (panic)

Panic v jednom jobu se zachytí, zaloguje a další tick proběhne normálně. **Sourozenecké joby nejsou ovlivněny** -- každý žije ve své goroutině s vlastním recover.

### Když job vrátí error

Loguje se na ERROR, ale ticker tiká dál. Předpokládá se, že joby jsou idempotentní -- vrátí error → další tick to zkusí znova → uspěje (případně).

### Validace při startu

Constructor `NewScheduler` odmítne duplicitní jméno, nulový interval, nil `Fn`. Chyba bublá až do `CreateApplication` a proces ani nestartuje (fail-fast). Lepší dovědět se to při deployi než ve 3 ráno při SIGTERM cleanupu.

### Joby běží paralelně s HTTP serverem

Scheduler i `server.Start` se spustí ve stejnou chvíli. Klidně se může stát, že první tick proběhne dřív, než server přijme první connection. **DB / repository volání jsou OK** (Wire DI je inicializované ještě před `Run`), ale **nesmíš v jobu volat vlastní HTTP API přes localhost** -- proběhne to jako závod.

### Multi-instance koordinace

Scheduler je in-process. Když nasadíš dvě repliky `serve`, **každá tickne**. Pro `DELETE expired_tokens` je to OK (idempotentní, dvojí DELETE nikomu neuškodí). Pro něco, co musí být provedeno **přesně jednou napříč clusterem**, přidej DB lock nebo přejdi na centrální scheduler (Temporal, Quartz, atd.).


## Co lze nastavit

Vše živé v `infrastructure/di/container_provider.go`, žádné env proměnné.

| Co | Kde | Default | Jak změnit |
|---|---|---|---|
| Které joby aplikace spouští | `provideSchedulerJobs()` | jen `cleanup:expired-refresh-tokens` | Přidej `scheduler.Job{}` do slice |
| Interval konkrétního jobu | Pole `Interval` v `scheduler.Job` | nastaveno v provideru (např. `1 * time.Hour`) | Změň hodnotu; restart procesu aby se aplikovala |
| Run-once-then-tick semantika | `runJob` v `infrastructure/scheduler/scheduler.go` | zapnuté pro všechny joby | Neměň -- záměrný invariant pro garantovaný tick za lifetime |
| Recovery panic / logging | Pevné v `runJob` | obojí zapnuté | Není konfigurovatelné -- bez toho by panic v jednom jobu zhasnul celý scheduler |
| Multi-instance koordinace | n/a | žádná | Pro single-execution napříč replikami přidej DB lock do `Fn` jobu (např. SELECT…FOR UPDATE pattern) |
