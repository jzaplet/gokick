---
layout: 'page'
uri: '/skills/gk-testing'
position: 20
slug: 'skills-gk-testing'
parent: 'skills-ship'
navTitle: 'gk-testing'
title: 'GK — Testing'
description: 'Testování v gokicku — testfx harness s reálnou SQLite DB, architektonické konformní testy (zz_audit/zz_gap) a quality gate (lint + arch + test + race + vitest + docs). Use when píšeš handler/repo test, nevíš jak rozjet DB v testu, řešíš proč ti spadl zz_audit/zz_gap, nebo co musí projít před commitem.'
name: 'gk-testing'
---

# GK — Testing

Jak se v gokicku testuje: integrační testy nad **reálnou** SQLite databází přes
`testfx`, samokontrolní testy hlídající architekturu (`zz_audit` / `zz_gap`) a
quality gate, který to celé před commitem prožene.

## What & when
- Sáhni sem, když: píšeš test pro command/query handler nebo repozitář,
  potřebuješ v testu reálnou DB + migrace + reálné repo/hasher/JWT, řešíš proč
  ti spadl `zz_audit_test.go` / `zz_gap_test.go`, nebo si ověřuješ, co musí
  projít před commitem (lint, arch, test, race, vitest, docs).
- NEtýká se: jak psát samotný handler (`/gk-commands`, `/gk-queries`), pravidla
  vrstev (`/gk-architecture`), ani DI wiring (`/gk-di`).

## For non-tech / juniors
Testy tu nejsou mockované — místo „předstíráme databázi" se pro každý test
založí **opravdová** malá SQLite databáze v dočasném souboru, naběhnou na ni
migrace a test píše a čte reálná data. Po testu se smaže sama. Tomu lešení se
říká `testfx` (test fixtures = připravené testovací prostředí).

Druhá skupina testů jsou „hlídači architektury". Projekt má pravidlo „tahle
vrstva nesmí sahat na tamtu". Tyhle testy projdou zdrojový kód jako text a
selžou v momentě, kdy někdo pravidlo poruší — jsou pojmenované `zz_…`, aby se
v seznamu řadily nakonec.

A nakonec „quality gate" — jeden příkaz (`make lint` + `make test`), který
ověří, že všechno (styl, architektura, testy) je v pořádku, než to pošleš dál.

## How it works

### testfx — reálná DB v testu
`app/internal/testfx/testfx.go`, import path `gokick/app/internal/testfx`.
`testfx.New(t, dbPath)` otevře izolovanou SQLite na `dbPath`, spustí migrace a
vrátí `*Fixture` s reálnými implementacemi (`Users`, `Tokens`, `Jobs`, `Hasher`,
`Jwt`, `DB`). DB se zavře automaticky přes `t.Cleanup`. Logger je tichý
(`io.Discard`). Užitečné helpery na `*Fixture`:
- `SeedUser(t, nickname, password, role)` / `SeedRefreshToken(t, userID, expiresAt)` — naplnění dat
- `AssertTokenCount(t, n)` — kontrola počtu řádků v `refresh_tokens`
- `NewBuses()` — postaví Command/Query/EventBus přesně jako `container_provider` (plný middleware chain)
- `ExecCommand[R](ctx, cmdBus, name, cmd, fn)` — **sankcionovaný způsob**, jak v handler testu protáhnout command celým chainem (tx, audit, eventy). Handler balíček nesmí importovat `application/bus` přímo (arch-lint: smí jen `bus_middleware`), takže to běží přes testfx.

Mimo bus (přímé volání handleru) se eventy chytají přes
`shared.ContextWithEventCollector(ctx)` + `collector.Flush()` —
viz `app/application/user/command/create_user_test.go`.

### zz_audit / zz_gap — konformní / coverage testy
Sada kurátorovaných testů s prefixem `zz_` (řadí se nakonec), každý cituje
v hlavičkovém komentáři **claim-ID z ledgeru** (`overview-39`,
`infra-db-security-10`, …; ledger žije jen v komentářích, ne jako doc). Mají dvě
fyzické podoby:

1. **Parser walks** (`go/parser`) — projdou zdrojáky jako text a pinují pravidla vrstev:
   - `app/domain/zz_audit_test.go` — domain smí importovat jen stdlib + `uuid` + jiný `domain/` (overview-39).
   - `app/domain/zz_gap_test.go` — HTTP handler nesmí importovat `infrastructure/sqlite`, `infrastructure/security` ani `application/**/event` (overview-41).
2. **testfx-wired black-box testy** — postaví reálné prostředí a pinují konkrétní coverage claim, např. `app/infrastructure/sqlite/user/zz_gap_test.go` (DB-level `CHECK`/`UNIQUE` constraints přes raw insert).

**Proč existují vedle go-arch-lintu:** parser walks jsou **silnější než
arch-lint** tam, kde matice závislostí nepomůže — `presentation → infrastructure`
je *legální* hrana, takže arch-lint handleru nezabrání sáhnout přímo do
`sqlite`/`security`; `domain/zz_gap_test.go` ano. Mají i **anti-vacuity
kontroly** (pozitivní kontrola + re-anchoring cesty), aby rozbitý walk neprošel
„zeleně, ale nic nezkontroloval". testfx-wired `zz_` testy, co self-importují
svůj produkční balíček (vypadá to jako cyklus), jsou v `.go-arch-lint.yml` →
`excludeFiles`.

### Quality gate
`make test` = `yarn test` (vitest) + `go test ./app/... ./cmd/...`.
`make lint` = ESLint + `vue-tsc` (type-check) + `golangci-lint` + `make arch-check`
(go-arch-lint) + `format-check` (golines) + `documan-lint`.
CI (`.github/workflows/validate.yml`): `make install` → `make lint` → `make test`
→ `make build`, se `SKIP_DOCUMAN=1` (dokumentaci v CI validuje samostatný
`.github/workflows/documan.yml` přes `docker/documan/Dockerfile`).

## Recipe

### Napsat integrační test handleru / repozitáře
1. `fx := testfx.New(t, filepath.Join(t.TempDir(), "moje.db"))` — reálná DB + migrace.
2. Naplň data: `u := fx.SeedUser(t, "bob", "secret12", "user")`.
3. Postav handler s reálnými závislostmi z `fx` (`fx.Users`, `fx.Hasher`, …).
4. Voláš handler buď přímo (eventy přes `shared.ContextWithEventCollector` + `collector.Flush()`),
   nebo přes plný chain: `cmdBus, _, _ := fx.NewBuses()` + `testfx.ExecCommand[...](...)`.
5. Asertuj proti DB (`fx.Users.FindByNickname(...)`, `fx.AssertTokenCount(t, n)`).

### Než commitnu
1. `make format` — srovná styl (ESLint Stylistic + golines).
2. `make lint` — ESLint + tsc + golangci-lint + arch-check + format-check (+ documan lokálně).
3. `make test` — vitest + `go test`.
4. `go test -race ./app/... ./cmd/...` — **manuální** krok na souběh; **není**
   v `make test` ani v CI, ale spouští se lokálně před většími změnami.

## Invariants & pitfalls
- **testfx je test-only.** Import `app/internal/testfx` patří jen do `*_test.go`.
  Komponenta `testfx` v `.go-arch-lint.yml` smí wirovat reálnou infrastrukturu
  právě proto, že ji produkční kód nikdy neimportuje.
- **Handler testy přes bus jedou `testfx.ExecCommand`, ne `application/bus` přímo** —
  jinak spadne `make arch-check` (handler smí na `bus_middleware`, ne na `bus`).
- **Nepřejmenuj `zz_`-testy bez kontextu.** Ruší se na nich claim-ID a
  anti-vacuity kontroly; prefix `zz_` je záměrný (řazení nakonec).
- **Nový bounded context = nový `domain_<ctx>` + grant v `mayDependOn`** napříč
  konzumenty (`application`, `sqlite_repos`, `testfx`, …), jinak arch-check padá.
  Viz `/gk-architecture`.
- **`-race` se hlídá ručně.** Není ve `make test` ani v `validate.yml` —
  nezapomeň ho pustit u změn, co se dotýkají souběhu (collector per-request, worker).
- **Každý test má izolovanou DB** přes `t.TempDir()` — nesdílej cestu mezi testy,
  ať jdou paralelně bez „database is locked".

## Related
- Skills: `/gk-architecture` (vrstvy + go-arch-lint), `/gk-commands`, `/gk-queries`
  (struktura handlerů), `/gk-repositories` (`r.Conn(ctx)`, raw-pool výjimky), `/gk-di`.
- Docs: [Architecture](/framework/architecture) (§ go-arch-lint, cross-domain izolace).
- Kód: `app/internal/testfx/testfx.go`, `app/domain/zz_audit_test.go`,
  `app/domain/zz_gap_test.go`, `app/infrastructure/sqlite/user/zz_gap_test.go`,
  `.go-arch-lint.yml`, `.golangci.yml`, `Makefile` (`test`, `lint`, `arch-check`),
  `.github/workflows/validate.yml`.
