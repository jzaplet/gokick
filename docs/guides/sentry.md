---
layout: 'page'
uri: '/guides/sentry'
position: 5
slug: 'guides-sentry'
parent: 'guides'
navTitle: 'Sentry'
title: 'Sentry'
description: 'Nastavení error trackingu (Sentry) pro backend i frontend — projekty, DSN, dev vs prod delivery, deploy za Cloudflare, ověření.'
---

# Sentry

Praktický návod, jak zapnout error tracking. Co a jak se reportuje uvnitř (port `ErrorReporter`, obohacení eventu, statické vynucení logovací cesty) popisuje [Observability](/framework/infrastructure/observability); tady jde o **operátorské kroky**: co založit a které proměnné nastavit.

**Rozsah:** jen chyby a paniky (rozsah A) — žádné performance tracing ani session replay. Bez DSN je celá věc no-op: aplikace běží beze změny a neposílá nic. Můžeš tedy gokick provozovat úplně bez Sentry účtu a zapnout ho až později.

## Krok 1 — Založ dva projekty

Backend a frontend jsou **dva samostatné Sentry projekty** (jiný runtime, jiné stack traces), takže dostaneš **dva DSN**:

- BE projekt (platforma Go) → `APP_SENTRY_DSN`
- FE projekt (platforma Vue/Browser) → `APP_SENTRY_DSN_FRONTEND`

> DSN je **veřejná** hodnota (jen ingest endpoint, ne tajemství) — je v pořádku ji vystavit v HTML a commitnout do deploy configu. Tajný je naopak `SENTRY_AUTH_TOKEN` pro upload source map (viz [Source maps](#source-maps) níže).

## Krok 2 — Backend

Backend čte Sentry config jako **runtime env** binárky (ne přes `.env` build-time):

```env
APP_SENTRY_DSN=https://<key>@<org>.ingest.<region>.sentry.io/<project>
APP_SENTRY_ENVIRONMENT=production
```

`APP_SENTRY_ENVIRONMENT` je sdílené i s frontendem. Release verze se nastavuje sama z git tagu při buildu (viz [Release verzování](#release-verzování)) — `APP_SENTRY_RELEASE` přepiš jen výjimečně. Tyto tři proměnné čte `cmd/` ještě před `LoadConfig`, takže nejsou v `Config` struct — viz [Config → Sentry](/framework/infrastructure/config#sentry).

## Krok 3 — Frontend: dev vs prod delivery

Tohle je nejdůležitější část a zároveň místo, kde frontend Sentry nejčastěji **tiše nefunguje**. FE potřebuje DSN v prohlížeči, ale dostane se tam dvěma různými cestami podle prostředí:

| Prostředí | Jak DSN doputuje do SPA | Proměnná |
|---|---|---|
| **Dev** (Vite dev server) | **build-time** — zapečeno do bundlu při `vite` | `VITE_SENTRY_DSN`, `VITE_SENTRY_ENVIRONMENT` v `.env` |
| **Prod** (Docker image) | **runtime** — Go server injektuje hodnoty do `index.html` jako `<meta name="gokick:…">` tagy, SPA je čte přes `runtimeConfig.ts` | `APP_SENTRY_DSN_FRONTEND`, `APP_SENTRY_ENVIRONMENT` (runtime env kontejneru) |

**Proč to rozdělení existuje:** produkční image se buildí **jednou** a nasazuje do více prostředí. Kdyby se FE DSN pekl při buildu (`VITE_SENTRY_DSN`), musel by se build dělat per-prostředí a v praxi v prod image chybí → FE Sentry zůstane **tmavý** (přesně tahle past). Runtime injekce přes `<meta>` tagy řeší „build once, deploy many": stejný image, jiné env proměnné kontejneru.

V prod tedy **nenastavuj `VITE_SENTRY_DSN`** — nastav `APP_SENTRY_DSN_FRONTEND` na kontejneru:

```env
# produkční kontejner
APP_SENTRY_DSN_FRONTEND=https://<key>@<org>.ingest.<region>.sentry.io/<fe-project>
APP_SENTRY_ENVIRONMENT=production
```

> **Výjimka — release.** FE release verze se na rozdíl od DSN/environment/debug **nepřenáší** za běhu — zůstává zapečená při buildu přes `VITE_SENTRY_RELEASE` (= `VERSION` / git tag v Dockerfile). Image je per-verze, takže to dává smysl. Neočekávej, že ji nastavíš runtime env proměnnou.

### CSP

Prohlížeč by jinak odeslání eventů do Sentry zablokoval (`connect-src`). Server to řeší sám: když je FE DSN nastavený, `SecurityHeadersMiddleware` **automaticky přidá ingest origin** toho DSN do `connect-src`. Ručně do CSP nesaháš.

## Krok 4 — Za Cloudflare / reverse proxy

Na zachyceném eventu je `user.ip_address` = klientská IP. Aby to byla **skutečná** IP (a ne edge IP proxy), nastav `APP_TRUST_PROXY_HEADERS=true` — server pak čte `CF-Connecting-IP` / `X-Real-IP`.

> ⚠️ Tyto hlavičky jsou **podvrhnutelné**, pokud je origin dosažitelný napřímo. `APP_TRUST_PROXY_HEADERS=true` zapni **jen** za firewallem omezeným na rozsahy proxy (origin-lock). Plný postup je v [Config → APP_TRUST_PROXY_HEADERS & Cloudflare origin-lock](/framework/infrastructure/config#app_trust_proxy_headers--cloudflare-origin-lock).

## Co se reportuje

Sentry **není logovací cesta** — chodí sem jen neočekávaná selhání:

- **Backend:** recovered paniky (bus i HTTP `RecoveryMiddleware`) a terminálně selhané joby (worker, vyčerpané retries).
- **Frontend:** Vue chyby + unhandled promise rejections.

Běžné návratové chyby — validace, auth, 4xx — se **nehlásí nikdy** (jinak tracker utone v šumu). Detail pravidla + jak je to staticky vynucené viz [Observability → Sentry](/framework/infrastructure/observability#sentry--chyby--paniky).

Každý BE event navíc nese **uživatele** (id / nickname / role + klientská IP) a **request** (method / URL / User-Agent — pevný whitelist, nikdy syrové hlavičky). FE drží `Sentry.setUser` v zámku se session. Mechanika je v [Observability](/framework/infrastructure/observability#sentry--chyby--paniky).

## Release verzování

Verze se stampuje při buildu z git tagu — do binárky přes `-ldflags "-X main.release=<tag>"` a do SPA přes `VITE_SENTRY_RELEASE`. Sentry tak grupuje issues podle nasazené verze. Release workflow (`.github/workflows/release.yml`, na `v*` tag) to dělá automaticky. Lokálně `make build` bere `git describe --tags`.

## Ověření (debug mód)

Po nasazení ověř, že eventy skutečně dorazí:

1. Nastav `APP_SENTRY_DEBUG=true` (BE i FE čtou stejnou hodnotu; FE ji dostane přes meta tag).
2. **Backend:** `curl https://<tvuj-host>/debug/sentry` → 500. V BE projektu se objeví event „http: panic in GET /debug/sentry".
3. **Frontend:** v appce se zobrazí tlačítko pro vyvolání chyby → klikni → event v FE projektu.
4. **Vypni** `APP_SENTRY_DEBUG` (zpět na `false`). Aplikace při startu varuje, dokud je zapnutý — **nikdy ho nenechávej v produkci**.

## Source maps

Bez nahraných source map jsou FE stack traces minifikované (nečitelné). Až budeš chtít čitelné traces, přidej do buildu `@sentry/vite-plugin` + tajný `SENTRY_AUTH_TOKEN` (upload source map). Do té doby FE Sentry funguje, jen traces nejsou rozbalené. Sleduje to [Roadmap](/roadmap), Fáze 5.
