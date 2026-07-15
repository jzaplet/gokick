---
layout: 'page'
uri: '/skills/gk-frontend-forms'
position: 20
slug: 'skills-gk-frontend-forms'
parent: 'skills-frontend'
navTitle: 'gk-frontend-forms'
title: 'GK — Frontend formuláře (backend-authoritative)'
description: 'Formuláře ve Vue, které nevalidují na frontendu — jen pošlou data a propíšou chyby z backendu na konkrétní pole. Use when píšeš/upravuješ formulář, chyba z API se nezobrazuje u správného pole, nebo řešíš, kam patří validace a proč ji frontend nedělá.'
name: 'gk-frontend-forms'
---

# GK — Frontend formuláře (backend-authoritative)

Formulář na frontendu nic nevaliduje. Pošle data přes `authFetch`, a když backend vrátí chybu jako `{ "<pole>": "<hláška>" }`, vloží ji 1:1 do stejně pojmenovaného pole. Žádné `if` nad kódy chyb, žádné duplicitní pravidlo.

## What & when
- Sáhni sem, když **píšeš nebo upravuješ formulář** (login, change-password, create/edit usera) a potřebuješ vzor pro state, odeslání a zobrazení chyb.
- Sáhni sem, když **chyba z API nepadá k poli** (vidíš ji jen jako obecnou) nebo nevíš, proč frontend nevaliduje.
- Tohle NEpopisuje, **kde validace žije** (value objects v doméně) — to řeší `/gk-commands` a `/gk-entities`. Ani **jak se chyba mapuje na HTTP status** (400/401/403) — to řeší `/gk-errors`. Tady jde čistě o frontend stranu.

## For non-tech / juniors
Formulář si představ jako **podací okénko**: vyplníš lístek, pošleš ho a čekáš na razítko. Razítkuje **backend** — ten jediný rozhoduje, co je správně. Kdyby si pravidla hlídalo i okénko (frontend), dřív nebo později se obě verze rozejdou a navíc útočník okénko prostě obejde. Proto frontend nevaliduje vůbec — jen ukáže, co backend vrátil.

Trik je v tom, že backend pošle chybu **pojmenovanou stejně jako pole** ve formuláři (`{ "nickname": "..." }`). Frontend pak nemusí nic překládat: vezme celou odpověď a „vysype" ji do formuláře — co má klíč `nickname`, se ukáže u políčka nickname. Když uživatel políčko opraví, chyba u něj zmizí.

## How it works
Validace je **server-side**; frontend jen propisuje. Klíčový je vzor **kdo vlastní state** — jsou dva:

**1. Self-contained formulář** — komponenta vlastní všechno: `errors`, `isLoading`, `handleSubmit` i samotný `authFetch`. Vzor: `assets/app/Profile/Components/ChangePasswordForm.vue`. Použij, když formulář žije jen na jednom místě.

**2. Split (View vlastní state, formulář jen emituje)** — View drží `errors`/`isLoading` a volá `authFetch`; formulářová komponenta je čistě prezentační a vysílá `emit('submit' | 'cancel' | 'clearError', …)`. Vzor: `assets/app/Admin/Components/UserForm.vue` + Views `AdminUserCreateView.vue` / `AdminUserEditView.vue`. Použij, když jeden formulář sdílí víc obrazovek (create i edit) — logika je v každém View jiná, layout stejný.

**Datový tok (oba vzory stejný):**
- `errors` je `ref<TErrors>({})` — prázdný objekt = bez chyb, klíč existuje = pole má chybu.
- Odpověď `authFetch` je rozlišená podle `result.success` (`ApiSuccess` / `ApiError` v `assets/app-ui/Fetch/types/`); na chybě z backendu je v `result.data` přesně objekt `{ <pole>: <hláška> }` (transportní selhání a rozbité 2xx tělo chodí unionem taky jako chyba, ale s `{ message: … }` — viz `/gk-frontend-fetch`).
- `errors.value = result.data` — jedna řádka, žádné mapování. Klíče z backendu sedí 1:1 na typ `TErrors`.
- Render: per-field chyba → `<Input :error="errors.<pole>" />` (`assets/app-ui/Inputs/Input.vue`, příp. `Select.vue`); obecná → `<ErrorAlert :message="errors.general" />` (`assets/app-ui/Alerts/ErrorAlert.vue`).

**Pozor na `required`:** prop `required` na `<Input>` je **čistě vizuální** — vykreslí hvězdičku `*` u labelu, ale nepropíše HTML atribut `required` na `<input>`. Jediné, co se reálně dostane do DOM, je `type` — takže `type="email"` dá nativní kontrolu formátu, nic víc. Pravda přichází z backendu.

## Recipe
Přidáváš/upravuješ formulář (vzor self-contained):

1. **Typ chyb** v `types/XxxErrors.ts` — všechny klíče optional, vždy `general`, názvy 1:1 jako pole backendu:
   ```typescript
   export type ChangePasswordErrors = { general?: string; new_password?: string };
(a k větě „názvy 1:1 jako pole backendu" doplnit: „— jen ta, ke kterým backend field-chybu opravdu vrací; špatné staré heslo chodí jako AuthError do general")
   ```
2. **State** — `reactive` pro data, `ref` pro chyby a loading:
   ```typescript
   const form = reactive<ChangePasswordFormData>({ old_password: '', new_password: '' });
   const errors = ref<ChangePasswordErrors>({});
   const isLoading = ref(false);
   ```
3. **Submit** — vyčisti chyby, pošli, na chybě vysyp `result.data`:
   ```typescript
   const handleSubmit = async (): Promise<void> => {
       isLoading.value = true;
       errors.value = {};
       const result = await authFetch<null, ChangePasswordErrors, ChangePasswordFormData>('PUT', '/api/v1/profile/password', { body: form });
       isLoading.value = false;
       if (result.success === false) {
           errors.value = result.data; // backend klíče = klíče typu
           return;
       }
       success('Password changed.');
   };
   ```
4. **Čištění při editaci** — chyba u pole zmizí, jakmile do něj uživatel sáhne:
   ```typescript
   const clearFieldError = (field: keyof ChangePasswordErrors): void => {
       // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
       delete errors.value[field];
   };
   ```
   Na inputu: `@update:model-value="() => clearFieldError('new_password')"`.
5. **Render** — `<Input :error="errors.old_password" … />` + `<ErrorAlert :message="errors.general" />`.

**Varianta split (formulář na víc obrazovkách):** state a `authFetch` přesuň do View, formulářová komponenta dostane `errors`/`isLoading` přes props a místo `authFetch` vysílá `emit('submit', { ...form })`, `emit('cancel')`, `emit('clearError', field)`. View pak řeší, kam po úspěchu přesměrovat. Viz `UserForm.vue` + `AdminUserCreateView.vue`.

## Invariants & pitfalls
- **Nulová FE validace.** Single source of truth je doména. Frontend nikdy nekontroluje délku/formát/povinnost — jen `type` (nativní browser kontrola). Duplikát pravidel se rozejde a útočník ho obejde.
- **`result.success === false`, ne `!result.success`.** Projekt vynucuje explicitní boolean check (CLAUDE.md). Stejně `=== true` u `if`.
- **`authFetch` na chráněné, `apiFetch` na veřejné.** `authFetch` (z `@/app-ui/Auth`) navíc na 401 jednou auto-refreshne token — formulář se o auth stav nestará. Veřejné endpointy bez retry → `apiFetch` z `@/app-ui/Fetch`.
- **Klíče `TErrors` musí sedět na názvy polí backendu.** Pole, jehož jméno nemá v `TErrors` protějšek — a `ValidationError` s prázdným `Field` — spadne do `general` (fallback `body["general"]` v `app/presentation/http/response/response.go`). Když chyba „mizí" do alertu místo k poli, zkontroluj, že se klíče shodují.
- **Jedna chyba, jeden klíč.** Backend vrací první chybu, na kterou narazí — víc klíčů najednou nechodí. Nepiš frontend tak, že čeká kolekci chyb.

## Related
- `/gk-commands` (value objects = zdroj validačních hlášek), `/gk-errors` (mapování chyb na 400/401/403), `/gk-entities` (kde validace v doméně žije)
- Kód: `assets/app/Profile/Components/ChangePasswordForm.vue`, `assets/app/Admin/Components/UserForm.vue`, `assets/app/Admin/Views/AdminUserCreateView.vue` + `AdminUserEditView.vue`, `assets/app-ui/Inputs/Input.vue` + `Select.vue`, `assets/app-ui/Alerts/ErrorAlert.vue`, `assets/app-ui/Auth/authFetch.ts`, `app/presentation/http/response/response.go`
