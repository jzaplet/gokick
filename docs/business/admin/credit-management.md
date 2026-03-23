---
layout: 'page'
uri: '/business/admin/credit-management'
position: 50
slug: 'business-admin-credit-management'
parent: 'business-admin'
navTitle: 'Kredity'
title: 'Přidělování kreditů'
description: 'Ruční přidělování kreditů uživatelům - přehled zůstatků, přidání kreditů.'
---

# Přidělování kreditů

## Role
- admin

## Účel
Ruční přidělování kreditů uživatelům. Admin rozhoduje komu a kolik kreditů přidá.

## Prvky

### Seznam uživatelů se zůstatky
- Tabulka uživatelů:
  - **Přezdívka**
  - **Aktuální zůstatek kreditů**
  - **Tlačítko "Přidat kredity"**

### Dialog / formulář přidání kreditů
- **Počet kreditů** - číselné pole (kolik kreditů přidat)
- **Tlačítko potvrdit**
- **Tlačítko zrušit**

## Akce
1. Admin vidí přehled všech uživatelů a jejich zůstatků
2. Klikne na "Přidat kredity" u vybraného uživatele
3. Zadá počet kreditů k přidání
4. Potvrdí
5. Systém připíše kredity uživateli

## Toast notifikace
- **Úspěch**: `Kredity přidány` - po úspěšném přidání kreditů uživateli

## Business pravidla
- Kredity přiděluje výhradně admin, ručně
- Standardní příděl je 60 kreditů na rok, ale admin může přidat libovolný počet
- Kredity se **přičítají** k aktuálnímu zůstatku (ne přepisují)
- Strhávání kreditů probíhá pouze přes [Odpověď na požadavek](/business/admin/respond-request), ne na této obrazovce
