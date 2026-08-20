---
layout: 'page'
uri: '/skills/gk-i18n'
position: 35
slug: 'skills-gk-i18n'
parent: 'skills-domain'
navTitle: 'gk-i18n'
title: 'GK — i18n: klíče přes API, render na frontendu'
description: 'Jak se překládá: API posílá {key, params} místo vět, msgkey klíče v doménových chybách, t()/tm() na frontendu, jediný katalog locale/*.json + make i18n-gen, rozlišení jazyka (X-App-Lang → gk_lang cookie → users.lang → Accept-Language → en) a jak správně přidat nový text. Use when přidáváš uživatelský text, chybovou hlášku, nebo řešíš proč něco vypadlo anglicky/česky špatně.'
name: 'gk-i18n'
---

# GK — i18n: klíče přes API, render na frontendu

Žádný uživatelský text se nezapéká do kódu a **backend žádnou větu nerenderuje** —
API posílá **typované překladové klíče + parametry** a na text je převádí až
frontend, v aktivním jazyce UI. Angličtina je kanonická a fallback, čeština
plnohodnotná mutace.

## What & when
- Sáhni sem, když: přidáváš text, který uvidí uživatel (chybová hláška, label,
  toast); zakládáš nový překladový klíč; řešíš, proč odpověď přišla ve špatném
  jazyce; přidáváš další jazyk.
- NEtýká se: mapování chyb na HTTP status (`/gk-errors`), tvaru FE error objektů
  (`/gk-frontend-forms`), logování — logy jsou vždy locale-free (`/gk-logging`).

## For non-tech / juniors
Aplikace mluví anglicky a česky. Aby se žádná věta nezapomněla přeložit, nesmí
být nikde napsaná natvrdo — místo ní je v kódu **klíč** (`user.nickname_required`)
a věty k němu leží v **katalozích** (jeden JSON soubor na jazyk, pro oba konce
tentýž). Server větu nikdy neskládá: když něco selže, pošle jen klíč + parametry
a prohlížeč si větu vyrenderuje sám v jazyce, který má uživatel právě zapnutý —
proto se i chybové hlášky přepnou hned s přepínačem jazyka. Překlep v klíči
neprojde kompilací: na backendu jsou klíče generované konstanty, na frontendu typ
odvozený z generovaného katalogu.

## How it works
**Jediný zdroj překladů.** `locale/messages.en.json` + `locale/messages.cs.json`
(root repa, vedle `migrations/`) drží VŠECHNY klíče — UI texty i chybové hlášky —
ve FE syntaxi: `{param}` placeholdery, plurál = objekt CLDR forem (česky
one/few/many/other, anglicky one/other; `other` povinné). `make i18n-gen`
(`gk i18n`, `tools/gk/i18n/i18n.go`) z nich generuje všechny artefakty:
`app/domain/shared/msgkey/keys.gen.go` (Go konstanty, kterými se konstruují
chyby), FE katalogy `assets/app-ui/I18n/catalog/<lang>.ts` a — z názvů souborů
v `locale/` — celou jazykovou sadu: `app/domain/shared/lang_gen.go`,
`assets/app-ui/I18n/langs.gen.ts` a `assets/app-ui/I18n/catalogs.gen.ts`
(hlavička říká generated, DO NOT EDIT — edituje se JSON). `make i18n-check`
(v `make lint`) hlídá: formát klíčů, paritu klíčů + parametrů +
plurálovosti proti kanonickému katalogu, validní CLDR formy včetně každé
kategorie, kterou `Intl.PluralRules` jazyka umí vybrat, byte-freshness
artefaktů, union deadness (klíč musí být užit v Go přes `msgkey.X` NEBO
v `assets/` jako literál) a pokrytí placeholderů `Params` na Go i FE call
sites.

**Backend = klíče na drátu.** Doménové chyby nesou `Key msgkey.Key` + `Params`
(`/gk-errors`); `Responder` (`app/presentation/http/response/response.go`) z nich
staví tělo `{"<pole>|general": {"key": "...", "params": {...}}}` — wire struct
`response.ApiMessage`, gkts-generovaný TS typ
`assets/app-ui/Fetch/types/ApiMessage.ts`. Parametry jsou lowercase (`count`,
`role`, `detail`); `count` volí plurál. Panic 500 body je jeden statický literál
`{"general":{"key":"common.internal_error"}}`
(`app/presentation/http/middleware/recovery.go`) — recovery cesta nikdy
nespouští encoder a nepotřebuje jazyk.

**Jazyk requestu (zůstává, ale neslouží API tělům).** `LangMiddleware`
(`app/presentation/http/middleware/lang.go`) resolvuje `X-App-Lang` header →
cookie `gk_lang` → `Accept-Language` → `en`; `AuthMiddleware` navrch aplikuje
`users.lang` z JWT claimu, když klient nevolil explicitně (header/cookie).
Výsledné pořadí: **header → cookie → profil → browser → en**. Ctx jazyk slouží
variantám `<html lang>` SPA shellu (`app/presentation/http/handler/spa.go`),
stampování `runs.lang` (dispatcher z ctx, worker obnoví — vzor `tenant_id`) a
budoucímu server-rendered výstupu (maily z durable runs si vrátí BE renderer
krmený z týchž `locale/` JSON). `users.lang` je **nullable** — NULL =
„nevysloveno", nastavuje ho jen uživatel sám (`PUT /api/v1/profile/lang`).

**Frontend = jediný renderer.** Generované katalogy
`assets/app-ui/I18n/catalog/en.ts` (kanonický, čistá data) + `cs.ts` (typovaný
`TranslationCatalog` — parita = vue-tsc); typy `TranslationKey = keyof typeof
enCatalog` a `TranslationCatalog` deklaruje ručně psaný
`assets/app-ui/I18n/lang.ts`.
`useI18n()` dává `t(key, params?)` (interpolace `{name}`, plurály přes
`Intl.PluralRules` + param `count`), `locale` a `chooseLocale`. **`tm(message)`**
renderuje ApiMessage z API: vezme `{key, params}` a přeloží v aktivním jazyce —
neznámý klíč vrátí surový klíč (fallback) + report do Sentry. Protože se render
děje až při zobrazení, chybové hlášky sledují přepnutí jazyka. Fetch layer
posílá `X-App-Lang` centrálně (`buildHeaders`). Explicitní volba (LangSwitcher —
vlaječky) jde do čitelné cookie `gk_lang`; server podle ní (a podle URL
prefixu / Accept-Language) servíruje `<html lang>` variantu shellu. URL:
angličtina bare, čeština `/cs/…`; router guard drží kanonické prefixy a dělá
detekční redirect podle efektivního jazyka.

## Recipe
### Recipe: nový/upravený text (UI i chybová hláška)
1. Přidej/uprav klíč v OBOU katalozích `locale/messages.en.json` a
   `locale/messages.cs.json` (stejné `{param}` parametry; plurál = objekt CLDR
   forem, česky one/few/many/other, `other` vždy).
2. `make i18n-gen` — vygeneruje `msgkey.<Konstanta>` i oba TS katalogy.
3. Použij: BE `&shared.ValidationError{Field: "x", Key: msgkey.NováKonstanta,
   Params: map[string]any{"count": limit}}` (lowercase parametry; `count`
   zároveň volí plurál) — NEBO FE `t('users.created', { nickname })`.
4. `make i18n-check` — parita, freshness, deadness i pokrytí parametrů.

### Recipe: vyrenderovat API chybu na frontendu
1. Chybové hodnoty z API jsou `ApiMessage` (`{key, params}`), ne věty — typ
   `assets/app-ui/Fetch/types/ApiMessage.ts`.
2. `errors.value = result.data;` merge zůstává beze změny (`/gk-frontend-forms`);
   při zobrazení hodnotu přelož: `tm(errors.value.general)` /
   `<Input :error="tm(errors.nickname)" />`.
3. `tm()` nikdy nehodí výjimku: neznámý klíč zobrazí surový klíč a reportne do
   Sentry — viditelný `user.nickname_required` je lepší než prázdno.

### Recipe: přidat jazyk
Ručně se sahá na **dvě místa**, zbytek je generovaný.
1. Katalog `locale/messages.<jazyk>.json` (kompletní — parita se vynucuje;
   nezapomeň klíč `lang.<jazyk>` do **všech** katalogů) a `make i18n-gen`.
2. Vlaječka: ikona v `assets/app-ui/Icons/Flag<Xx>Icon.vue` + řádek v
   `langMeta` v `assets/app-ui/I18n/LangSwitcher.vue`. SVG se generovat nedá;
   `Record<Lang, LangMeta>` na to spadne ve vue-tsc, dokud řádek nedoplníš.

Vše ostatní si generátor odvodí z názvu katalogu: `shared.SupportedLangs` /
`ParseLang` / `DefaultLang`, FE `SUPPORTED_LANGS` / `Lang` / `CANONICAL_LANG`,
i mapa jazyk→katalog. Router prefix i shell varianty jedou z těch seznamů.
Ruční editace kteréhokoli artefaktu spadne na byte-freshness.

CLDR kategorie se necachují: `gk i18n` je při **každém** běhu (generate i
check) čte z Node `Intl.PluralRules`, takže brána chce přesně ty formy, které
renderer umí vybrat. Commitnutá tabulka by se porovnávala sama se sebou —
chybný záznam (ruční merge, starší ICU) by se schvaloval napořád a tiše přestal
vyžadovat formu, kvůli které kontrola existuje. Cenou je `node` na PATH i pro
`make i18n-check`; `make lint` ho stejně už potřebuje na eslint a vue-tsc.

## Invariants & pitfalls
- **Nikdy věta do literálu, nikdy render na serveru.** Chyby nesou `Key`;
  API posílá `{key, params}`; render dělá výhradně FE (`t()`/`tm()`). Hardcoded
  string v `t()` neprojde typem, mrtvý klíč neprojde `i18n-check`.
- **Generované soubory needituj.** `keys.gen.go`, `lang_gen.go`,
  `catalog/<lang>.ts`, `langs.gen.ts` i `catalogs.gen.ts` vznikají
  z `locale/` — edit JSON + `make i18n-gen`; ruční zásah spadne
  na byte-freshness gate.
- **Parametry lowercase.** `count`, `role`, `detail` — `{Count}` neprojde
  validací katalogu. `count` musí být číslo (volí CLDR formu); plurálový klíč
  volaný bez něj vrací surový klíč + Sentry report (FE), chybějící `Params`
  na Go call site chytá `i18n-check` staticky.
- **Logy, `Error()` a CLI jsou locale-free** — vrací surový klíč. Nepřekládej
  nic, co jde do logu (`/gk-logging` klíče ≠ překladové klíče).
- **`users.lang` mění jen uživatel sám.** Admin/platform update cesty ho
  nechávají být (jako `tenant_id`); NULL = rozhoduje browser.
- **`X-App-Lang` je v CORS allow-listu** — přidáš-li další custom header, patří
  do `Access-Control-Allow-Headers` v `cors.go`.

## Related
- Sousední skills: `/gk-errors` (typy chyb, které klíče nesou + wire tvar),
  `/gk-entities` (value objects vracející ValidationError), `/gk-frontend-forms`
  (kam hláška dopadne ve formuláři), `/gk-frontend-fetch` (syntetizovaná
  `{ general }` selhání), `/gk-runs` (proč má run vlastní lang), `/gk-logging`
  (locale-free logy)
- Kód: `locale/` (katalogy — jediný zdroj), `tools/gk/i18n/i18n.go` (generátor +
  gaty), `app/domain/shared/lang.go`, `app/domain/shared/msgkey/keys.gen.go`,
  `app/presentation/http/response/response.go` (`ApiMessage`),
  `app/presentation/http/middleware/lang.go`,
  `app/presentation/http/middleware/recovery.go` (statický panic body),
  `app/presentation/http/handler/spa.go` (`<html lang>` shell),
  `assets/app-ui/I18n/` (t/tm, LangSwitcher, generované katalogy)
