---
layout: 'page'
uri: '/skills/gk-errors'
position: 30
slug: 'skills-gk-errors'
parent: 'skills-domain'
navTitle: 'gk-errors'
title: 'GK — Doménové chyby a mapování na HTTP status'
description: 'Doménové chyby (ValidationError / AuthError / PermissionError) a jejich automatické mapování na HTTP status (400 / 401 / 403 / 500) bez cross-layer importu. Use when potřebuješ vrátit z handleru / handleru commandu chybu se správným HTTP kódem, nebo nevíš, proč ti něco padá na 500 místo 400.'
name: 'gk-errors'
---

# GK — Doménové chyby a mapování na HTTP status

Doménová vrstva pojmenuje druh chyby (validace / přihlášení / oprávnění) a `response/`
balíček z toho sám odvodí HTTP status — domain přitom o HTTP vůbec nic neví.

## What & when
- Sáhni sem, když: vracíš z value objectu nebo command/query handleru chybu a chceš
  správný HTTP kód; řešíš proč klient dostal `500` místo `400`; chceš, aby se hláška
  trefila do konkrétního pole formuláře.
- NEtýká se: domain events / `EventCollector` (druhá půlka `errors-events.md`) → `/gk-domain-events`.
  Kdo `PermissionError` produkuje a vynucuje (`AuthorizeMiddleware`) → `/gk-bus`.
  Kde value objecty `ValidationError` vrací → `/gk-entities`.

## For non-tech / juniors
Když něco selže, aplikace nevrací jen „chyba“ — vrací **druh** chyby. Jsou tři typy:

- **ValidationError** — uživatel poslal špatná data (prázdný nick, krátké heslo). → HTTP `400`.
- **AuthError** — uživatel není přihlášený (chybí / propadlý / neplatný token). → HTTP `401`.
- **PermissionError** — přihlášený je, ale na tuhle akci nemá právo. → HTTP `403`.

Cokoli jiného (rozbitá databáze, neočekávaný pád) je „naše chyba“ → HTTP `500`.
Trik je, že doménový kód jen řekne „tohle je validační chyba“ a HTTP vrstva si status
dopočítá sama. Doménový kód tak nemusí znát web — dá se použít i z CLI nebo testů.

## How it works
Tři error typy žijí v `app/domain/shared/errors.go`. Každý má metodu `HTTPStatus() int`:

```go
func (e *ValidationError) HTTPStatus() int    { return 400 }  // + ErrorField() string
func (e *AuthError) HTTPStatus() int          { return 401 }
func (e *PermissionError) HTTPStatus() int     { return 403 }
```

Mapování řeší `app/presentation/http/response/response.go`. Funguje na **duck typing**
(„kachní typování“ — když to umí `HTTPStatus()`, bere se to jako HTTP chyba; nezáleží
na názvu typu, jen na metodě). Klíčové je, kdo koho zná:

- `response` si definuje vlastní rozhraní (`interface` — kontrakt „co metoda umí“)
  `HTTPError` a `FieldError`.
- Domain ty metody jen **implementuje** a `response` **nikdy neimportuje** → drží se
  pravidlo vrstev (domain nezávisí na ničem nad sebou).

`HandleError(ctx, w, err)` — metoda na injektovaném `*response.Responder` (`response.go`) — udělá `errors.As(err, &httpErr)`:
- sedne na `HTTPError` → vrátí `httpErr.HTTPStatus()` a tělo odvozené z chyby;
- nesedne → vrátí `500` a generickou `errInternal` (klíč `common.internal_error`). Skutečná
  chyba se **nezveřejní** (neúniknou repo/panika interní detaily); dohledá se v logu přes
  `trace_id`.

**Tělo nese klíč, ne větu:** hodnota je `response.ApiMessage` —
`{"key": "<katalog.klíč>", "params": {...}?}`. Chyby s `MessageParams()`
(`KeyedError`: ValidationError, AuthError, PermissionError, MessageError) přispějí
klíč + parametry; cokoli jiného pošle svůj `Error()` text jako klíč bez parametrů
(fallback pro debug endpointy). Render dělá frontend přes `tm()` (`/gk-i18n`).

Routování hlášky do pole (`Error(ctx, w, status, err)`, `response.go`): jen
`ValidationError` má `ErrorField()`. Když je `Field` neprázdné → JSON klíč = jméno pole
(`{"nickname": {"key": "...", "params": {...}}}`). Prázdné pole nebo
`AuthError`/`PermissionError` → `{"general": {"key": "..."}}`.

## Recipe
### Recipe: vrátit chybu z handleru se správným statusem
1. **Doménová / bus chyba** (z value objectu, command/query handleru přes bus) →
   `h.resp.HandleError(r.Context(), w, err)` (metoda na injektovaném `*response.Responder`). Status se odvodí z typu chyby automaticky.
2. **Selhání dekódování requestu** (rozbitý JSON v těle) → `request.DecodeJSON` vrací typovaný `*shared.MessageError` s vlastním statusem (`413` pro nadměrné tělo, `400` pro ostatní selhání), takže i tady stačí `h.resp.HandleError(r.Context(), w, err)` (viz `admin_users.go:126`). Status ručně neurčuj.
3. Konstruuj chybu vždy **pointerem**: `&shared.AuthError{Key: msgkey.AuthRefreshTokenMissing}`
   (viz `auth.go:117`), nikdy hodnotou — viz pitfall níže.

### Recipe: validační chyba navázaná na konkrétní pole
1. Ve value objectu vrať `&shared.ValidationError{Field: "nickname", Key: msgkey.UserNicknameRequired}`
   (hláška je překladový klíč z katalogů, ne věta — viz `/gk-i18n`).
2. Handler ji propustí přes `h.resp.HandleError(r.Context(), w, err)` → klient dostane
   `400` a `{"nickname": {"key": "user.nickname_required"}}`. Frontend hodnotu
   vyrenderuje přes `tm()` v aktivním jazyce UI a ukáže u inputu — server žádnou
   větu neskládá.

## Invariants & pitfalls
- **Vždy pointer (`&`).** Metody jsou na `*T` (`func (e *ValidationError) HTTPStatus()`).
  `errors.As` v `HandleError` proto sedne **jen na pointer**. Když vrátíš hodnotu
  (`shared.ValidationError{...}` místo `&...`), tiše propadne na `500`. Toto je nejčastější
  past — z `400`/`401`/`403` se stane `500`.
- **Domain neimportuje `response`.** Směr duck typingu je daný: `response` definuje
  rozhraní, domain implementuje metody. Obrácený import poruší pravidlo vrstev
  (`make arch-check` ho zachytí).
- **`500` nikdy neúnikne interní detail.** Non-`HTTPError` se klientovi vrací jako
  generická `errInternal`; nikdy nevracej raw repo/panic hlášku přímo klientovi.
- **Permission se neřeší v handleru.** `PermissionError` produkuje `AuthorizeMiddleware`
  v busu, ne HTTP role guard. Handler ji jen propustí přes `HandleError`. Detaily `/gk-bus`.

## Related
- Sousední skills: `/gk-domain-events` (druhá půlka `errors-events.md`), `/gk-bus`
  (`PermissionError` z `AuthorizeMiddleware`), `/gk-entities` (value objecty vracející `ValidationError`)
- Kód: `app/domain/shared/errors.go`, `app/presentation/http/response/response.go`,
  handlery `app/presentation/http/handler/` (`auth.go`, `admin_users.go`)
