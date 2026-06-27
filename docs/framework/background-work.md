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
- Potřebuješ **něco zavolat ven** (e-mail/SMTP, webhook, cizí API) nebo máš **dlouhou** práci (velký import, velký report)? → **durable run** (běží mimo transakci, takže I/O nezamkne DB; po pádu pokračuje).
- Máš **krátkou práci jen nad vlastní DB** (přepočítat agregaci, kaskádní update), co musí **přežít restart**? → **job**.
- Chceš něco spouštět **opakovaně po čase** (úklid, synchronizace)? → **scheduler**.


## Přehledová tabulka

| Způsob | Kdy běží | V transakci? | Použij na |
|---|---|---|---|
| **Command** | hned v requestu | ✅ ano | ulož/uprav/smaž data (vše naráz, nebo nic) |
| **Doménový event** | hned po uložení dat | ❌ ne (po uložení) | „stalo se X" → reakce, kterou command nezná |
| **Job** | zvlášť na pozadí | ✅ ano (krátká) | **rychlá práce jen nad vlastní DB** (přepočet, kaskádní update) — **NIC ven** |
| **Durable run** | zvlášť, **dlouho** | ❌ **NE** | **volání ven** (e-mail, API, webhook) **nebo** dlouhá práce (import, report) |
| **Scheduler** | opakovaně po čase | ❌ ne | úklid, synchronizace; velkou práci → zařaď job/run |


## Proč „v transakci, nebo ne" tolik záleží

SQLite umí **psát jen z jednoho místa naráz**. Transakce si ten zámek na zápis vezme **hned na začátku** a drží ho až do konce. Z toho plyne jedno tvrdé pravidlo:

> **Uvnitř transakce NIKDY nevolej ven** (síť, e-mail/SMTP, cizí API). Drželo by to zámek na zápis po celou dobu toho volání — SMTP může viset, API request klidně 5 minut → celá databáze mezitím nemůže nic zapsat. Volání ven patří **mimo transakci** (durable run).

- **Command a Job transakci chtějí** — jsou **krátké a sahají jen do vlastní DB**. Drží zámek jen chvilku a za to získají jistotu, že se uloží buď všechno, nebo nic. (Pozor: job běží celý uvnitř transakce, takže v něm **nesmí být volání ven** — to je častý omyl; patří do runu.)
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
