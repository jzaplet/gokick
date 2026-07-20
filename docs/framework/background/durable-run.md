---
layout: 'page'
uri: '/framework/durable-run'
position: 30
slug: 'framework-durable-run'
parent: 'framework-background'
navTitle: 'Durable run'
title: 'Durable run'
description: 'Cesta durable runu — dlouho běžící práce (velký import, generování velkého reportu, dávkové zpracování), která běží MIMO transakci, průběžně ukládá postup a po pádu workeru pokračuje od posledního uloženého kroku.'
---

# Durable run

Engine pro práci, která běží **mimo transakci** — protože buď **trvá dlouho**, nebo **volá ven** (a obojí by v transakci zamklo databázi). Hodí se na velký import/export dat, generování velkého reportu/PDF, dávkové zpracování mnoha položek — a stejně tak na **volání pomalých nebo nespolehlivých cizích služeb** (e-mail/SMTP, webhook, cizí API). Taková práce po každém kroku **uloží, kde skončila** (checkpoint), a když proces mezitím spadne, jiný worker ji převezme a **pokračuje od posledního uloženého kroku**.

![Run flow — dlouhá práce, co po pádu pokračuje od posledně](../files/run-flow.svg)

> Přehled toku. Návod (jak napsat run, cancel) → `/gk-runs`; kdy durable run vs fire-and-forget run vs scheduler → [Background work](/framework/overview).


## K čemu to je

Když práce **trvá dlouho a nesmí začít od nuly**, kdyby proces spadl. Obyčejný [fire-and-forget run](/framework/fire-and-forget) běží sice taky mimo transakci, ale **nepamatuje si postup** — když spadne, příště začne od začátku. Run si po každém kroku ukládá, kde je (checkpoint), takže ho jiný worker **převezme a pokračuje od posledního uloženého kroku**. To je jediný rozdíl mezi fire-and-forget a durable runem: **run má checkpoint, fire-and-forget run ne** — jinak je to ten samý engine.

> Proč obojí běží **mimo transakci**: dlouhá práce — nebo **jakékoli volání ven** (e-mail/SMTP, cizí API) — by v transakci držela SQLite write-lock celou tu dobu (SMTP může viset, API request 5 minut) → **zamkla by celou databázi** pro všechny ostatní. Proto je outside-tx vynucené pro oba tvary.


## Jak to teče

1. **Zařazení** — z command handleru zavoláš `RunDispatcher.Enqueue(kind, maxRetries, payload)`. Zápis do fronty se připojí ke **stejné transakci** jako tvoje uložení dat — buď se uloží obojí, nebo nic (jako u fire-and-forget runu).
2. **Převzetí** — worker si práci atomicky vezme a označí ji jako „dělám na tom" (nikdo jiný ji mezitím nevezme).
3. **Běh mimo transakci** — handler dostane vstupní data a dosavadní postup (`r.State`). Běží **bez transakce**, klidně minuty až hodiny. (Otevřít transakci **nechtěně** uvnitř handleru framework nedovolí — fail-closed; vědomou krátkou transakci na atomický zápis pár řádků umožňuje `shared.WithTx`.)
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

`maxRetries` (kolik logických pokusů) je povinné. Run handler **nesmí nechtěně otevřít transakci** (implicitní/omylem otevřenou transakci framework fail-closed odmítne); když potřebuješ pár zápisů uložit atomicky, použij `shared.WithTx(ctx, fn)` — krátkou transakci, kterou si sám ohraničíš (zapiš pár řádků, commitni, pokračuj). Drž ji krátkou a bez pomalého/externího I/O — stejné pravidlo jako uvnitř command handleru (jinak background run transakci nemá).


## Související

- [Fire-and-forget](/framework/fire-and-forget) — fire-and-forget tvar téhož enginu (bez checkpointu).
- [Background work](/framework/overview) — co kdy použít + proč background práce běží mimo transakci.
- [Command](/framework/command) — transakce, ke které se zařazení připojí.
- Skilly: `/gk-runs`, `/gk-repositories`.
