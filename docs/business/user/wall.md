---
layout: 'page'
uri: '/business/user/wall'
position: 10
slug: 'business-user-wall'
parent: 'business-user'
navTitle: 'Zeď požadavků'
title: 'Zeď požadavků - Pohled uživatele'
description: 'Hlavní obrazovka s požadavky všech uživatelů - karty, stavy, odpovědi.'
---

# Zeď požadavků - Pohled uživatele

## Role
- user

## Účel
Hlavní obrazovka aplikace. Zobrazuje seznam všech požadavků od všech uživatelů. Uživatel vidí stav svých i cizích požadavků.

## Prvky

### Seznam požadavků
- Požadavky zobrazeny jako karty/řádky seřazené od nejnovějšího
- Každý požadavek zobrazuje:
  - **Přezdívka autora** - kdo požadavek zadal
  - **Text požadavku** - popis co uživatel chce
  - **Stav** - vizuálně odlišený: Nový / Hotovo / Nelze sehnat
  - **Datum vytvoření**
  - **Odpověď admina** (pokud existuje) - textová zpráva od admina
  - **Stržené kredity** (pokud stav = Hotovo) - kolik kreditů bylo strženo

### Tlačítko "Nový požadavek"
- Odkaz na formulář nového požadavku

## Akce
1. Uživatel prohlíží zeď požadavků
2. Kliknutím na požadavek může zobrazit jeho detail
3. Kliknutím na "Nový požadavek" přejde na formulář

## Business pravidla
- Zobrazují se požadavky **všech** uživatelů
- Uživatel nemůže své požadavky editovat ani mazat
- Stav požadavku je vizuálně odlišen (barvy/ikony): Nový, Hotovo, Nelze sehnat
