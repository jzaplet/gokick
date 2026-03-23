---
layout: 'page'
uri: '/business/admin/wall'
position: 10
slug: 'business-admin-wall'
parent: 'business-admin'
navTitle: 'Zeď požadavků'
title: 'Zeď požadavků - Pohled admina'
description: 'Zeď požadavků s tlačítkem odpovědět a filtrováním podle stavu.'
---

# Zeď požadavků - Pohled admina

## Role
- admin

## Účel
Hlavní obrazovka pro admina. Stejná zeď požadavků jako pro uživatele, ale s přidaným tlačítkem pro odpověď na požadavky.

## Prvky

### Seznam požadavků
- Stejné zobrazení jako u uživatelské zdi (viz [Zeď požadavků - Pohled uživatele](/business/user/wall))
- Každý požadavek navíc obsahuje:
  - **Tlačítko "Odpovědět"** - u požadavků ve stavu Nový

### Filtrování/řazení
- Možnost filtrovat podle stavu (Nový / Hotovo / Nelze sehnat)

## Akce
1. Admin prohlíží zeď požadavků
2. U požadavku ve stavu "Nový" klikne na "Odpovědět"
3. Je přesměrován na obrazovku [Odpověď na požadavek](/business/admin/respond-request)

## Business pravidla
- Admin vidí požadavky všech uživatelů
- Tlačítko "Odpovědět" se zobrazuje pouze u požadavků ve stavu **Nový**
- Admin vidí u každého požadavku zůstatek kreditů jeho autora (pro rozhodování o stržení)
