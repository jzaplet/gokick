---
layout: 'page'
uri: '/business/shared/login'
position: 20
slug: 'business-shared-login'
parent: 'business-shared'
navTitle: 'Login'
title: 'Login - Přihlašovací obrazovka'
description: 'Přihlášení uživatele - výběr ze seznamu, zadání hesla, JWT autentizace.'
---

# Login - Přihlašovací obrazovka

## Role
- Nepřihlášený uživatel

## Účel
Přihlášení uživatele do aplikace. Uživatel se nevybírá přes klasický formulář s loginem, ale klikne na sebe v seznamu a zadá heslo.

## Prvky

### Seznam uživatelů
- Seznam všech aktivních uživatelů zobrazených jako klikatelné karty/tlačítka
- Každý uživatel zobrazen svou **přezdívkou**
- Po kliknutí na uživatele se zobrazí pole pro zadání hesla

### Formulář hesla (po výběru uživatele)
- **Pole pro heslo** - textové pole typu password
- **Tlačítko přihlásit**
- **Tlačítko zpět** - návrat na výběr uživatele

## Akce
1. Uživatel klikne na svou přezdívku v seznamu
2. Zobrazí se pole pro zadání hesla
3. Uživatel zadá heslo a klikne na "Přihlásit"
4. Systém ověří heslo a vydá JWT token
5. Po úspěšném přihlášení je uživatel přesměrován na zeď požadavků

## Toast notifikace
- **Úspěch**: `Přihlášení úspěšné` - po úspěšném přihlášení (na zdi po redirectu)
- **Error**: `Nesprávné heslo` - po zadání špatného hesla
- **Warning**: `Byli jste odhlášeni` - po redirectu z jiné stránky kvůli expirovanému JWT

## Business pravidla
- Seznam zobrazuje pouze aktivní uživatele (ne smazané/deaktivované)
- Po neúspěšném přihlášení se zobrazí chybová hláška, uživatel zůstává na obrazovce hesla
- Po úspěšném přihlášení systém vydá JWT token obsahující ID uživatele a jeho roli (admin/user)
