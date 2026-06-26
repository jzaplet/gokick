---
layout: 'page'
uri: '/framework/background-work'
position: 45
slug: 'framework-background-work'
parent: 'framework'
navTitle: 'Background work'
title: 'Background work — co kdy použít'
description: 'Jak vybrat mezi command, doménovým eventem, jobem, durable runem a schedulerem — a proč jen durable run běží mimo transakci (jinak by dlouhá práce zamkla celou SQLite).'
---

# Background work — co kdy použít

gokick má pět způsobů, jak něco „udělat". Vyber podle dvou otázek: **běží to hned v requestu, nebo zvlášť na pozadí?** a **smí to běžet v transakci?** Špatná volba buď ztratí práci (něco, co mělo přežít restart, běželo jen tak), nebo **zamkne celou databázi** (dlouhá práce v transakci drží zámek na zápis).

![Co kdy použít na práci na pozadí](files/background-work.svg)

> Detailní toky: [Command flow](/framework/command-flow) · [Event flow](/framework/event-flow) · [Job flow](/framework/job-flow) · [Run flow](/framework/run-flow) · [Scheduler flow](/framework/scheduler-flow). Návody: `/gk-commands`, `/gk-domain-events`, `/gk-jobs`, `/gk-runs`, `/gk-scheduler`.


## Rychlá volba

- Potřebuješ **uložit/upravit data** v reakci na akci uživatele, **vše naráz nebo nic**? → **command**.
- Chceš, aby na něco **zareagoval někdo další** („stalo se X"), aniž to ten command musí vědět? → **doménový event**.
- Máš **krátkou** práci, co se **může nepovést a má se zopakovat** (e-mail, cizí API) a musí **přežít restart**? → **job**.
- Máš **dlouhou** práci (velký import, velký report), co musí **pokračovat po pádu**? → **durable run**.
- Chceš něco spouštět **opakovaně po čase** (úklid, synchronizace)? → **scheduler**.


## Přehledová tabulka

| Způsob | Kdy běží | V transakci? | Použij na |
|---|---|---|---|
| **Command** | hned v requestu | ✅ ano | ulož/uprav/smaž data (vše naráz, nebo nic) |
| **Doménový event** | hned po uložení dat | ❌ ne (po uložení) | „stalo se X" → reakce, kterou command nezná |
| **Job** | zvlášť na pozadí | ✅ ano (krátká) | krátká nespolehlivá práce (e-mail, API), přežije restart |
| **Durable run** | zvlášť, **dlouho** | ❌ **NE** | dlouhá práce (import, report), pokračuje po pádu |
| **Scheduler** | opakovaně po čase | ❌ ne | úklid, synchronizace; velkou práci → zařaď job/run |


## Proč „v transakci, nebo ne" tolik záleží

SQLite umí **psát jen z jednoho místa naráz**. Transakce si ten zámek na zápis vezme **hned na začátku** a drží ho až do konce. Z toho plyne jednoduché pravidlo:

- **Command a Job transakci chtějí** — jsou **krátké**. Drží zámek jen chvilku a za to získají jistotu, že se uloží buď všechno, nebo nic.
- **Durable run transakci mít nesmí** — je **dlouhý**. Kdyby dlouhá práce držela zámek na zápis minuty/hodiny, **nikdo jiný by mezitím nemohl nic zapsat** — celá appka by zamrzla. Proto run běží mimo transakci a postup ukládá **krátkými zápisy** mezi kroky.
- **Doménový event** běží **až po uložení dat** (po commitu), takže už není v transakci toho commandu. Běží ale pořád v requestu — **pomalý event brzdí odpověď uživateli**, takže těžkou navazující práci radši **zařaď jako job/run**.
- **Scheduler** běží jen tak na pozadí, bez transakce. Krátký úklid v transakci je v pohodě; **velkou práci zařaď jako job/run**, ať dlouhá transakce nezamkne databázi.

### Že run nesmí otevřít transakci, hlídá framework

Aby dlouhá práce omylem nezamkla databázi, je to pravidlo **vynucené**, ne jen napsané:

1. **Za běhu** — když se uvnitř run handleru někdo pokusí otevřít transakci, **selže to s jasnou chybou** (framework označí ten běh jako „bez transakce" a otevření odmítne).
2. **Při buildu** — test projde kód run handleru a **shodí build**, kdyby tam transakce byla.

Run tedy ukládá postup přes `Checkpointer`; když opravdu potřebuješ něco uložit atomicky, **zařaď na to command nebo job** — ten poběží ve své vlastní krátké transakci mimo run.


## Související

- Toky: [Command flow](/framework/command-flow), [Event flow](/framework/event-flow), [Job flow](/framework/job-flow), [Run flow](/framework/run-flow), [Scheduler flow](/framework/scheduler-flow).
- Skilly: `/gk-commands`, `/gk-domain-events`, `/gk-jobs`, `/gk-runs`, `/gk-scheduler`, `/gk-bus`.
