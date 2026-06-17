---
layout: 'page'
uri: '/roadmap'
position: 40
slug: 'roadmap'
navTitle: '🗺️ Roadmap (template)'
title: 'Roadmap'
description: 'Fázovaný plán vývoje — každá fáze má stav, své Briefs a Issues a adresované ADR. (Template — vyplň podle svého projektu.)'
---

# Roadmap

Interní fázovaný plán. Každá fáze má **stav** a odkazuje na své **Briefs** (rozvahy *za*
rozhodnutím — viz [Briefs](/briefs)) a **Issues** (konkrétní práce — viz [Issues](/issues)).
Fáze jsou číslované `F1, F2, …` a staví na sobě.

> **Tohle je template.** Nahraď placeholder fáze svými. (Reálná roadmapa tohoto
> skeletonu je [Roadmap (GoKick)](/framework/gokick-roadmap) ve Frameworku.)

**Legenda:** ✅ hotovo · 🟡 probíhá · 🔴 nezačato. U briefu ✅ = reviewed/accepted.


## ADRs

Klíčová, zafixovaná rozhodnutí, na kterých fáze staví — produktová i architektonická,
jeden záznam = jedno rozhodnutí. Celý seznam a statusy ve [ADRs](/adrs).

- [ADR-0000 — Project scope](/adrs/0000-adr-project-scope) — co projekt je a co není


## Fáze


### 🔴 F1 — `<název fáze>`

`<Jednou větou, co fáze dodá a proč. Na čem staví.>`

- 🔴 [Plán: example brief](/briefs/0000-example-brief) — design & rationale
- 🔴 [Feature template](/issues/features/0000-feature-template) — feature tasks

**Adresuje:** [ADR-0000 — Project scope](/adrs/0000-adr-project-scope)


### 🔴 F2 — `<název fáze>`

`<Jednou větou. Staví na F1.>`

- 🔴 [Plán: example brief](/briefs/0000-example-brief) — design & rationale
- 🔴 [Bug template](/issues/bugs/0000-bug-template) — defect register

**Adresuje:** [ADR-0000 — Project scope](/adrs/0000-adr-project-scope)
