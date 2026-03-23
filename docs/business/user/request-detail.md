---
layout: 'page'
uri: '/business/user/request-detail'
position: 30
slug: 'business-user-request-detail'
parent: 'business-user'
navTitle: 'Detail požadavku'
title: 'Detail požadavku - Pohled uživatele'
description: 'Zobrazení kompletního detailu požadavku včetně odpovědi admina.'
---

# Detail požadavku - Pohled uživatele

## Role
- user

## Účel
Zobrazení kompletního detailu jednoho požadavku včetně odpovědi admina.

## Prvky

### Informace o požadavku
- **Přezdívka autora**
- **Datum vytvoření**
- **Text požadavku**
- **Stav** - Nový / Hotovo / Nelze sehnat

### Odpověď admina (pokud existuje)
- **Textová zpráva** - komentář admina k požadavku
- **Stržené kredity** (pokud stav = Hotovo) - kolik kreditů bylo strženo za tento požadavek

### Navigace
- **Tlačítko zpět** - návrat na zeď požadavků

## Akce
1. Uživatel si prohlíží detail požadavku
2. Klikne zpět pro návrat na zeď

## Business pravidla
- Uživatel vidí detail jakéhokoliv požadavku (svého i cizího)
- Uživatel nemá žádné editační možnosti na této obrazovce
- Stržené kredity se zobrazují pouze u požadavků ve stavu Hotovo
