---
layout: 'page'
uri: '/business/admin/user-form'
position: 40
slug: 'business-admin-user-form'
parent: 'business-admin'
navTitle: 'Formulář uživatele'
title: 'Formulář uživatele (vytvoření / editace)'
description: 'Vytvoření nového uživatele nebo editace existujícího - přezdívka, heslo, email, role.'
---

# Formulář uživatele (vytvoření / editace)

## Role
- admin

## Účel
Vytvoření nového uživatele nebo editace existujícího.

## Prvky

### Formulář
- **Přezdívka** - povinné, textové pole
- **Heslo** - povinné při vytváření, nepovinné při editaci (prázdné = beze změny)
- **Email** - nepovinné, textové pole
- **Role** - výběr: admin / user
- **Tlačítko uložit**
- **Tlačítko zrušit** - návrat na seznam uživatelů

## Akce

### Vytvoření
1. Admin vyplní přezdívku, heslo, volitelně email, vybere roli
2. Klikne na "Uložit"
3. Systém vytvoří uživatele s 0 kredity
4. Admin je přesměrován na seznam uživatelů

### Editace
1. Formulář je předvyplněný aktuálními údaji (heslo prázdné)
2. Admin upraví potřebné údaje
3. Klikne na "Uložit"
4. Systém aktualizuje údaje

## Toast notifikace
- **Úspěch**: `Uživatel vytvořen` - po vytvoření nového uživatele (na seznamu po redirectu)
- **Úspěch**: `Uživatel uložen` - po editaci existujícího uživatele (na seznamu po redirectu)
- **Error**: `Přezdívka již existuje` - při pokusu uložit duplicitní přezdívku

## Business pravidla
- Přezdívka musí být unikátní
- Při vytváření je heslo povinné
- Při editaci prázdné heslo znamená beze změny
- Nový uživatel se vytváří s **0 kredity** (admin pak přidělí ručně přes správu kreditů)
