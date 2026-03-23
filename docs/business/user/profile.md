---
layout: 'page'
uri: '/business/user/profile'
position: 40
slug: 'business-user-profile'
parent: 'business-user'
navTitle: 'Profil'
title: 'Profil uživatele'
description: 'Základní profil uživatele s možností změny hesla.'
---

# Profil uživatele

## Role
- user

## Účel
Základní profil uživatele s možností změny hesla.

## Prvky

### Informace o uživateli (readonly)
- **Přezdívka**
- **Email** (pokud je vyplněn)
- **Zůstatek kreditů**

### Formulář změny hesla
- **Aktuální heslo** - pole typu password
- **Nové heslo** - pole typu password
- **Potvrzení nového hesla** - pole typu password
- **Tlačítko uložit**

## Akce
1. Uživatel vyplní aktuální heslo a nové heslo (2x)
2. Klikne na "Uložit"
3. Systém ověří aktuální heslo a uloží nové

## Toast notifikace
- **Úspěch**: `Heslo změněno` - po úspěšné změně hesla
- **Error**: `Nesprávné aktuální heslo` - při zadání špatného aktuálního hesla
- **Error**: `Hesla se neshodují` - když se nové heslo a potvrzení liší

## Business pravidla
- Uživatel nemůže měnit svou přezdívku ani email (to dělá admin)
- Pro změnu hesla musí zadat správné aktuální heslo
- Nové heslo a jeho potvrzení se musí shodovat
