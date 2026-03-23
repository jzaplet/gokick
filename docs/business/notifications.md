---
layout: 'page'
uri: '/business/notifications'
position: 40
slug: 'business-notifications'
parent: 'business'
navTitle: 'Notifikace'
title: 'Emailové notifikace'
description: 'Systém emailových notifikací - odpověď na požadavek, stržení kreditů.'
---

# Emailové notifikace

## Účel
Systém emailových notifikací informující uživatele o změnách v jejich požadavcích.

## Notifikace

### Odpověď na požadavek
- **Kdy**: Admin odpoví na požadavek uživatele (změní stav na Hotovo nebo Nelze sehnat)
- **Komu**: Autorovi požadavku (pokud má vyplněný email)
- **Obsah**:
  - Název/text požadavku
  - Nový stav (Hotovo / Nelze sehnat)
  - Počet stržených kreditů (pokud Hotovo)
  - Zpráva od admina (pokud byla zadána)
  - Aktuální zůstatek kreditů uživatele

## Business pravidla
- Notifikace se odesílá **pouze uživatelům s vyplněným emailem**
- Pokud uživatel nemá email, notifikace se neodesílá (bez chyby)
- Uživatel se o odpovědi dozví buď z emailu, nebo když se podívá na zeď/detail požadavku
