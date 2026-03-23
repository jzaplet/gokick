---
layout: 'page'
uri: '/business/admin/users'
position: 30
slug: 'business-admin-users'
parent: 'business-admin'
navTitle: 'Správa uživatelů'
title: 'Správa uživatelů'
description: 'Seznam všech uživatelů s CRUD operacemi - přidání, editace, smazání.'
---

# Správa uživatelů

## Role
- admin

## Účel
Seznam všech uživatelů s možností CRUD operací.

## Prvky

### Seznam uživatelů
- Tabulka/seznam všech uživatelů s údaji:
  - **Přezdívka**
  - **Email** (pokud vyplněn)
  - **Zůstatek kreditů**
  - **Role** (admin / user)
- U každého uživatele:
  - **Tlačítko editovat** - přechod na formulář editace
  - **Tlačítko smazat** - smazání uživatele (s potvrzením)

### Tlačítko "Přidat uživatele"
- Přechod na formulář vytvoření nového uživatele

## Akce
1. Admin prohlíží seznam uživatelů
2. Klikne na "Přidat uživatele" → formulář vytvoření
3. Klikne na "Editovat" u uživatele → formulář editace
4. Klikne na "Smazat" → potvrzovací dialog → smazání

## Toast notifikace
- **Úspěch**: `Uživatel smazán` - po úspěšném smazání uživatele

## Business pravidla
- Pouze admin má přístup k této obrazovce
- Smazání uživatele vyžaduje potvrzení (dialog "Opravdu smazat?")
- Admin nemůže smazat sám sebe
