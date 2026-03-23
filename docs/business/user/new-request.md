---
layout: 'page'
uri: '/business/user/new-request'
position: 20
slug: 'business-user-new-request'
parent: 'business-user'
navTitle: 'Nový požadavek'
title: 'Nový požadavek'
description: 'Formulář pro zadání nového požadavku.'
---

# Nový požadavek

## Role
- user

## Účel
Formulář pro zadání nového požadavku.

## Prvky

### Formulář
- **Text požadavku** - textarea, uživatel popíše co chce
- **Tlačítko odeslat**

## Akce
1. Uživatel vyplní text požadavku
2. Klikne na "Odeslat"
3. Požadavek se uloží se stavem "Nový"
4. Uživatel je přesměrován zpět na zeď požadavků

## Toast notifikace
- **Úspěch**: `Požadavek odeslán` - po úspěšném odeslání (na zdi po redirectu)
- **Error**: `Vyplňte text požadavku` - při pokusu odeslat prázdný formulář

## Business pravidla
- Po odeslání požadavek nelze editovat ani zrušit
- Požadavek se vytvoří se stavem **Nový**
- Požadavek je přiřazen přihlášenému uživateli
- Textové pole je povinné (nelze odeslat prázdný požadavek)
