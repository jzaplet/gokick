---
layout: 'page'
uri: '/business/shared/layout'
position: 10
slug: 'business-shared-layout'
parent: 'business-shared'
navTitle: 'Layout'
title: 'Layout - Společný layout aplikace'
description: 'Společný rámec všech obrazovek - status bar s kredity, navigace, odhlášení.'
---

# Layout - Společný layout aplikace

## Role
- Všechny role (admin, user)

## Účel
Společný rámec všech obrazovek po přihlášení. Obsahuje navigaci a status bar s kredity.

## Prvky

### Status bar (horní lišta)
- **Přezdívka uživatele** - zobrazení aktuálně přihlášeného uživatele
- **Zůstatek kreditů** - aktuální počet kreditů uživatele (číslo, může být i záporné)
- **Tlačítko odhlášení**

### Navigace
- **Zeď požadavků** - odkaz na hlavní zeď
- **Nový požadavek** - odkaz na formulář nového požadavku (pouze role user)
- **Můj profil** - odkaz na profil uživatele
- **Správa uživatelů** - odkaz na seznam uživatelů (pouze role admin)

## Business pravidla
- Zůstatek kreditů se zobrazuje vždy a aktualizuje se po každé změně (stržení, přidělení)
- Navigační položky se zobrazují podle role přihlášeného uživatele
- JWT token se validuje při každém requestu; po expiraci je uživatel přesměrován na login
