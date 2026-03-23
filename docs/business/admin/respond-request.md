---
layout: 'page'
uri: '/business/admin/respond-request'
position: 20
slug: 'business-admin-respond-request'
parent: 'business-admin'
navTitle: 'Odpověď na požadavek'
title: 'Odpověď na požadavek'
description: 'Formulář pro odpověď admina - stav, stržení kreditů, textová zpráva.'
---

# Odpověď na požadavek

## Role
- admin

## Účel
Obrazovka kde admin reaguje na požadavek uživatele - označí zda požadavek splnil nebo ne, strhne kredity a přidá zprávu.

## Prvky

### Informace o požadavku (readonly)
- **Přezdívka autora**
- **Zůstatek kreditů autora** - aktuální stav kreditů uživatele
- **Text požadavku**
- **Datum vytvoření**

### Formulář odpovědi
- **Stav** - výběr: Hotovo / Nelze sehnat
- **Počet kreditů ke stržení** - číselné pole (zobrazuje se pouze při stavu Hotovo)
- **Textová zpráva** - textarea pro komentář admina (nepovinné)
- **Tlačítko odeslat**
- **Tlačítko zrušit** - návrat na zeď bez uložení

## Akce
1. Admin vidí detail požadavku a zůstatek kreditů autora
2. Vybere stav (Hotovo / Nelze sehnat)
3. Pokud vybral Hotovo, zadá počet kreditů ke stržení
4. Volitelně přidá textovou zprávu
5. Klikne na "Odeslat"
6. Systém změní stav požadavku, strhne kredity (pokud Hotovo) a uloží zprávu
7. Odešle emailovou notifikaci autorovi požadavku (pokud má vyplněný email)
8. Admin je přesměrován zpět na zeď

## Toast notifikace
- **Úspěch**: `Odpověď odeslána` - po úspěšném odeslání odpovědi (na zdi po redirectu)

## Business pravidla
- Pole "Počet kreditů" se zobrazuje pouze při stavu **Hotovo**
- Při stavu **Nelze sehnat** se kredity nestrhávají
- Systém **neblokuje** stržení kreditů do mínusu - admin vidí zůstatek a rozhoduje sám
- Po odeslání odpovědi se stav požadavku změní z Nový na vybraný stav
- Odpovědět lze pouze na požadavky ve stavu Nový
- Po odpovědi se odešle emailová notifikace autorovi (pokud má email)
