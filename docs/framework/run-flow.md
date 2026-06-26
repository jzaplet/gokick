---
layout: 'page'
uri: '/framework/run-flow'
position: 75
slug: 'framework-run-flow'
parent: 'framework'
navTitle: 'Run flow'
title: 'Run flow'
description: 'Cesta durable runu — dlouho běžící práce (velký import, generování velkého reportu, dávkové zpracování), která běží MIMO transakci, průběžně ukládá postup a po pádu workeru pokračuje od posledního uloženého kroku.'
---

# Run flow

Engine pro **dlouho běžící práci, která umí pokračovat po pádu**. Hodí se na věci, co trvají dlouho — velký import nebo export dat, generování velkého reportu/PDF, dávkové zpracování mnoha položek. Taková práce běží **mimo transakci**, po každém kroku si **uloží, kde skončila** (checkpoint), a když proces mezitím spadne, jiný worker ji převezme a **pokračuje od posledního uloženého kroku**.

![Run flow — dlouhá práce, co po pádu pokračuje od posledně](files/run-flow.svg)

> Přehled toku. Návod (jak napsat run, cancel) → `/gk-runs`; kdy run vs job vs scheduler → [Background work](/framework/background-work).


## K čemu to je

Když práce **trvá dlouho a nesmí začít od nuly**, kdyby proces spadl. Obyčejný [job](/framework/job-flow) by nestačil ze dvou důvodů:

1. **Job běží v transakci.** Dlouhá práce v transakci drží zámek na zápis do databáze celou tu dobu → **zamkne celou databázi** pro všechny ostatní. Run proto běží mimo transakci.
2. **Job si nepamatuje postup.** Když spadne, začne příště od začátku. Run si po každém kroku ukládá, kde je, takže pokračuje.


## Jak to teče

1. **Zařazení** — z command handleru zavoláš `RunDispatcher.Enqueue(kind, maxRetries, payload)`. Zápis do fronty se připojí ke **stejné transakci** jako tvoje uložení dat — buď se uloží obojí, nebo nic (jako u jobu).
2. **Převzetí** — worker si práci atomicky vezme a označí ji jako „dělám na tom" (nikdo jiný ji mezitím nevezme).
3. **Běh mimo transakci** — handler dostane vstupní data a dosavadní postup (`r.State`). Běží **bez transakce**, klidně minuty až hodiny. (Otevřít transakci uvnitř handleru framework **nedovolí** — fail-closed.)
4. **Checkpoint** — handler po každém kroku zavolá `ck.Save(postup)` — krátký zápis, kde skončil. Když mezitím o práci přišel (převzal ji jiný worker), `Save` to oznámí a handler skončí.
5. **Heartbeat** — vedle běží malý hlídač, který každou chvíli dá databázi vědět, že práce pořád žije. Dokud žije, nikdo jiný ji nepřevezme.
6. **Dokončení** — povedlo se → označí hotovo; selhalo a zbývají pokusy → zkusí znovu s odstupem (backoff); došly pokusy → označí selhání. Práce, o kterou worker přišel nebo byla zrušena, se **nikdy nedokončí napůl**.
7. **Po pádu** — když worker spadne, práce po chvíli „zestárne" a jiný worker ji **převezme a pokračuje od posledního uloženého kroku**. Pojistka proti nekonečnému padání práci po pár pokusech ukončí.

Stav se odvozuje ze sloupců v DB (hotovo / selhalo / zrušeno / kdy naposledy žila), žádný zvláštní „status" sloupec.


## Příklad

Handler je obyčejná funkce. Musí umět **pokračovat z dosavadního postupu** a **snést, že se krok po pádu zopakuje** (idempotence):

```go
func ZpracujDavku(ctx context.Context, r *run.Run, ck run.Checkpointer) error {
    postup := nactiPostup(r.State)        // prázdné → začni od začátku
    for !postup.Hotovo {
        zpracujDalsiKrok(ctx, &postup)    // jeden krok — bez transakce, jde přerušit
        if err := ck.Save(ctx, uloz(postup)); err != nil {
            return err                     // o práci jsme přišli → skonči
        }
    }
    return nil
}

// zařazení z command handleru:
shared.RunDispatcherFromContext(ctx).Enqueue(ctx, "import:velky", 3, payload)
```

`maxRetries` (kolik logických pokusů) je povinné. Run handler **nesmí otevřít transakci** (framework to hlídá); když potřebuješ něco uložit atomicky, **zařaď na to command nebo job** — ten poběží ve své vlastní krátké transakci mimo run.


## Související

- [Job flow](/framework/job-flow) — kratší sourozenec (běží v transakci).
- [Background work](/framework/background-work) — co kdy použít + proč jen runy běží mimo transakci.
- [Command flow](/framework/command-flow) — transakce, ke které se zařazení připojí.
- Skilly: `/gk-runs`, `/gk-jobs`, `/gk-repositories`.
