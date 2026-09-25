---
layout: 'page'
uri: '/skills/gk-testing'
position: 20
slug: 'skills-gk-testing'
parent: 'skills-ship'
navTitle: 'gk-testing'
title: 'GK — Testing'
description: 'Testování v gokicku — testfx harness s reálnou DB na zvoleném adaptéru (APP_DB_DRIVER), kontraktní testy repozitářů, architektonické konformní testy (zz_audit/zz_gap/zz_nosqlite) a quality gate (lint + arch + test + race + vitest + docs). Use when píšeš handler/repo test, nevíš jak rozjet DB v testu, řešíš proč ti spadl zz_audit/zz_gap/zz_nosqlite, nebo co musí projít před commitem.'
name: 'gk-testing'
---

# GK — Testing

Jak se v gokicku testuje: integrační testy nad **reálnou** databází přes
`testfx` (na adaptéru, který vybere `APP_DB_DRIVER` — dnes SQLite, s Postgres
adaptérem stejný test beze změny i na Postgresu), samokontrolní testy hlídající
architekturu (`zz_audit` / `zz_gap` / `zz_nosqlite`) a quality gate, který to
celé před commitem prožene.

## What & when
- Sáhni sem, když: píšeš test pro command/query handler nebo repozitář,
  potřebuješ v testu reálnou DB + migrace + reálné repo/hasher/JWT, řešíš proč
  ti spadl `zz_audit_test.go` / `zz_gap_test.go`, nebo si ověřuješ, co musí
  projít před commitem (lint, arch, test, race, vitest, docs).
- NEtýká se: jak psát samotný handler (`/gk-commands`, `/gk-queries`), pravidla
  vrstev (`/gk-architecture`), ani DI wiring (`/gk-di`).

## For non-tech / juniors
Testy tu nejsou mockované — místo „předstíráme databázi" se pro každý test
založí **opravdová** malá databáze, naběhnou na ni migrace a test píše a čte
reálná data. Po testu se smaže sama. Tomu lešení se říká `testfx` (test
fixtures = připravené testovací prostředí). Test sám neví, jaká databáze pod ním
běží — to určuje proměnná `APP_DB_DRIVER` stejně jako u aplikace.

Druhá skupina testů jsou „hlídači architektury". Projekt má pravidlo „tahle
vrstva nesmí sahat na tamtu". Tyhle testy projdou zdrojový kód jako text a
selžou v momentě, kdy někdo pravidlo poruší — jsou pojmenované `zz_…`, aby se
v seznamu řadily nakonec.

A nakonec „quality gate" — jeden příkaz (`make lint` + `make test`), který
ověří, že všechno (styl, architektura, testy) je v pořádku, než to pošleš dál.

## How it works

### Kde testy žijí (layout)

- **Go testy** (`_test.go`) — VŽDY vedle svého balíčku (`app/**`, `cmd/**`, `tools/gk/**`), nikdy v `tests/`.
  Jazykový idiom, ne preference: white-box testy potřebují package scope
  (neexportované symboly) a `zz_*` konformanční gaty skenují vlastní adresář
  přes `runtime.Caller`. Přesun by je rozbil.
- **`tests/assets/`** — FE vitest testy (jediné místo, kde testy žijí mimo
  testovaný kód; Vue SFC nemá package-local konvenci).
- **`tests/e2e/`** — shell E2E harness durable-run engine (`make e2e`,
  proces-lifecycle: crash/drain/at-least-once/terminal).


### testfx — reálná DB v testu
`app/internal/testfx/`, import path `gokick/app/internal/testfx`.
`testfx.New(t)` (nebo `testfx.NewMultitenant(t)`) založí testu vlastní
databázi na adaptéru z `APP_DB_DRIVER` (default `sqlite`; čte se **jen**
z prostředí procesu, nikdy z `.env`), spustí migrace a vrátí `*Fixture`
s reálnými porty ze **stejného** `persistence.Store` jako produkce: `Users`,
`PlatformUsers`, `Tokens`, `Runs`, `Tenants`, `PlatformTenants`, `Audit`, `Tx`
(`shared.Transactor`), plus `Hasher` a `Jwt`. DB zmizí sama přes `t.Cleanup`.
Logger je tichý (`io.Discard`). Test nikdy nevidí konkrétní adaptér ani cestu
k souboru. Každý backend má vlastní opener s build tagem
(`app/internal/testfx/sqlite.go` je `//go:build !nosqlite`).

Helpery na `*Fixture`:
- **Seed:** `SeedUser`, `SeedUserInTenant`, `SeedTenant`, `SeedTenantWithPlan`, `SeedRunInTenant`, `SeedRefreshToken`, `MarkRunCompleted`. Zapisují se `testfx.SystemCtx()` — v systémové rovině (`shared.PlaneSystem`), protože seedování je setup napříč tenanty; na Postgresu by tenantová rovina řádek cizího tenanta odmítla (RLS `WITH CHECK`). Stejný ctx použij pro jiný fixture zápis, který musí sáhnout mimo aktivního tenanta.
- **Rovina, když voláš handler nebo repozitář bez busu:** dej mu ctx, jaký by mu dal bus — `testfx.TenantCtx(tenantID)` (práce za jednoho tenanta, jako handler runu), `testfx.PlatformCtx()` (platformní handler), `testfx.SystemCtx()` (CLI, seeder, inspekce řádku cizího tenanta). Holý `context.Background()` je tenantová rovina výchozího tenanta: na SQLite projde cokoli, na Postgresu RLS ukáže jen řádky výchozího tenanta.
- **Stav, který porty vyrobit neumí** (`app/internal/testfx/raw.go`): `SetUserActive`, `SetUserLockedUntil`, `ForceExpireLease`, `SetLeaseFromNow(t, id, d)` (vůči hodinám **databáze**, ms přesně), `StealLease`, `MakeRunDue`, `ForceRunCompleted` / `ForceRunFailed`, `SetRunReclaims` / `SetRunParks`.
- **Čtení mimo porty:** `Count(t, table, where, args…)`, `AuditEntry(t, id)`, `AssertTokenCount(t, n)`.
- **Constraint testy:** `RawExec(query, args…)` (přenositelné SQL s `?`) + `Violated(err)` → `testfx.NotNull` / `Unique` / `Check` / `ForeignKey`, klasifikované z kódu chyby driveru, ne z textu hlášky.
- **Busy:** `NewBuses()` postaví Command/Query/EventBus přesně jako `container_provider` (plný middleware chain), `NewSystemBus()` jeho CLI obdobu.
- `ExecCommand[R](ctx, cmdBus, name, cmd, fn)` — **sankcionovaný způsob**, jak v handler testu protáhnout command celým chainem (tx, audit, eventy). Handler balíček nesmí importovat `application/bus` přímo (arch-lint: komponenta `application` nemá grant na `bus` ani na `bus_middleware`), takže to běží přes testfx.
- **Dva procesy nad jednou DB:** `fx.Replica(t)` otevře databázi fixture podruhé, s vlastními pooly. Vrátí `persistence.Store`, jaký by měla druhá replika `serve`. Na testy zámků mezi replikami (scheduler) a souběhu dvou procesů.
- **Souběžný zápis uprostřed editu:** `fx.RaceEdit(t, edit, concurrent)` spustí edit nad hasherem, jehož první `Hash` edit zastaví. Edit uživatele hashuje nové heslo mezi načtením řádku a zápisem, takže stojí přesně mezi čtením a zápisem. Do té mezery helper pustí `concurrent`, po chvíli edit pustí dál a vrátí chyby obou (`app/application/user/command/concurrent_edit_test.go`).
- **Jen JWT, bez DB:** `jwtfx.New(t, accessExp)` z `gokick/app/internal/testfx/jwtfx`. Middleware testy tak nelinkují žádný DB adaptér.

**Výběr driveru v testu:** `testfx.ActiveDriver()`. Test, který pinuje chování jednoho adaptéru, patří do balíčku toho adaptéru, který má `TestMain` s `testfx.MainFor(m, database.Driver<Adaptér>)`, takže při jiném driveru neběží vůbec (Postgres adaptér: `app/infrastructure/postgres/main_test.go`). Test souběhu mimo balíček adaptéru, který na SQLite nemá smysl (SQLite zápisy serializuje, dva commandy se nikdy nepotkají), začne `t.Skip` s důvodem, když `testfx.ActiveDriver()` není Postgres (`app/infrastructure/di/bus_retry_test.go`). Test, který na SQLite projde triviálně a na Postgresu hlídá skutečný souběh, běží na obou (`app/internal/repotest/tenant/delete_race_test.go`).

### Postgres: `make test-pg` a harness `pgfx`
Na Postgresu běží **celá suite**: `testfx.New(t)` postaví Store z Postgres repozitářů nad klonem šablony. Navíc tam běží vlastní testy adaptéru (`app/infrastructure/postgres/`: manager, migrátor, kontrola rolí, `BaseRepository`, zlatý test řazení a **RLS sada** `rls_test.go`, která izolaci tenantů ověřuje přímo v databázi, bez kódu repozitářů).
- `make test-pg` nahodí compose službu `db-test` (Postgres 18 z `docker/postgres/Dockerfile`, data v RAM, `fsync=off`, žádný publikovaný port — připojí se na IP kontejneru) a spustí `go test -tags nosqlite ./app/... ./cmd/...` s `APP_DB_DRIVER=postgres`. S nastaveným `APP_TEST_DB_URL` (superuser DSN) poběží proti jinému clusteru; ten musí mít role z `docker/postgres/initdb/01-roles.sh` s výchozími hesly. V CI to dělá job `postgres tests`.
- **Pozor na cache testů.** `go test` si pamatuje prošlé balíčky a znovu je nepouští (`(cached)`). O databázi nic neví, takže změna Postgres image nebo init skriptu cache neinvaliduje. Při ověřování proti DB pouštěj `-count=1`. Testy souběhu opakuj s `-count=20` (a `-race`).
- **Harness** `app/internal/testfx/pgfx`: `pgfx.New(t)` vrátí testu vlastní databázi, **klon šablony** s hotovým schématem (šablona `gokick_tpl_<hash migrací>` vznikne jednou pod advisory lockem, i když `go test` pouští balíčky paralelně; klon trvá milisekundy), a po testu ji smaže. `pgfx.NewEmpty(t)` dá prázdnou DB pro testy migrací. `DB.Config()` vrátí `*config.Config` s DSN všech tří rolí; `DB.AdminURL` slouží testům, které ověřují, že aplikace superuserovi odmítne sloužit.

### Kontraktní testy repozitářů
`app/internal/repotest/<ctx>/` (`audit`, `run`, `tenant`, `token`, `user`, `tx`)
testují **porty** (`user.Repository`, `run.Repository`, …), ne konkrétní
adaptér. Napíšou se jednou a poběží na každém adaptéru, který `testfx` umí
otevřít. V adaptéru (`app/infrastructure/sqlite/`) zůstávají jen testy jeho
vlastních specifik (DSN pragmata, collation, pool cap, goose Down, gate testy
nad jeho SQL).

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
   - `app/domain/zz_gap_test.go` — HTTP handler nesmí importovat DB adaptér (`infrastructure/sqlite`, `infrastructure/postgres`, `infrastructure/persistence`), `infrastructure/security` ani `application/**/event` (overview-41).
   - `app/zz_nosqlite_test.go` — viz „Žádná SQLite při přepnutém adaptéru" níže.
   - `app/zz_tenant_test.go` — tenant conformance nad SQL **obou** adaptérů (dotaz nad tenant-owned tabulkou má `tenant_id` nebo marker; INSERT se `tenant_id` volá write guard); `app/zz_sqltime_test.go` (SQLite, `julianday`) a `app/zz_pgtime_test.go` (Postgres, jen `statement_timestamp()`) hlídají hodiny DB; `app/zz_pgsystem_test.go` drží allow-list Postgres metod na systémové roli. Čtou jen zdroj, takže běží v každém běhu.
2. **testfx-wired black-box testy** — postaví reálné prostředí a pinují konkrétní coverage claim, např. `app/internal/repotest/user/zz_gap_test.go` (DB-level `CHECK`/`UNIQUE` constraints přes raw insert).

### Žádná SQLite při přepnutém adaptéru (trojitá pojistka)
1. **Kompilace:** celý SQLite adaptér a oba jeho openery (`app/infrastructure/persistence/sqlite.go`, `app/internal/testfx/sqlite.go`) jsou za `//go:build !nosqlite`. `make nosqlite-check` (součást `make lint`) spustí `golangci-lint --build-tags nosqlite` a ověří, že `go list -tags nosqlite -test -deps` neobsahuje ncruces ani adaptér — žádný balíček, ani testovací.
2. **Staticky:** `app/zz_nosqlite_test.go` hlídá, že SQLite import je jen v souborech s tagem, každý soubor adaptéru tag má, každý testovací balíček adaptéru má `TestMain` s `MainFor` a žádný test ani fixture mimo tagované soubory nenese SQLite dialekt (`julianday(`, `strftime(`, `datetime(`, `PRAGMA`, `sqlite_master`, `INSERT OR …`, cestu `*.db`).
3. **Runtime:** `testfx` otevře jen adaptér z `APP_DB_DRIVER`; v buildu bez něj test hlasitě selže, místo aby tiše běžel jinde.

**Proč existují vedle go-arch-lintu:** parser walks jsou **silnější než
arch-lint** tam, kde matice závislostí nepomůže — `presentation → infrastructure`
je *legální* hrana, takže arch-lint handleru nezabrání sáhnout přímo do
`sqlite`/`security`; `domain/zz_gap_test.go` ano. Mají i **anti-vacuity
kontroly** (pozitivní kontrola + re-anchoring cesty), aby rozbitý walk neprošel
„zeleně, ale nic nezkontroloval". testfx-wired `zz_` testy, co self-importují
svůj produkční balíček (vypadá to jako cyklus), jsou v `.go-arch-lint.yml` →
`excludeFiles`.

### Quality gate
`make test` = `yarn test` (vitest, v CI job `lint + test + build`) + `go test ./app/... ./cmd/...` + `cd tools/gk && go test ./...` (dev nástroje tsgen/boundary/errfields/docpaths jsou vlastní modul, takže je `./app/...` nepokrývá).
`make lint` = ESLint + `vue-tsc` (type-check) + `knip` (dead code) + `golangci-lint` + `make arch-check` (go-arch-lint) + `nosqlite-check` (build bez SQLite, viz výše) + `format-check` (golines) + `ts-check` (Go→TS parita typů) + `boundary-check` (wire DTO hranice) + `errfields-check` (parita chybových polí) + `i18n-check` (parita překladových katalogů a freshness generovaných artefaktů) + `docpaths-check` (každá cesta a `/gk-*` odkaz v docs/skills musí existovat) + `documan-lint`.
CI (`.github/workflows/validate.yml`): job `validate` = `make install` → `make lint` → `make test` → `make build`, se `SKIP_DOCUMAN=1` (dokumentaci v CI validuje samostatný `.github/workflows/documan.yml` přes `docker/documan/Dockerfile`); paralelní job `postgres tests` spouští `make test-pg` (stejný Docker image a init skript jako lokálně) a job `e2e` spouští `make e2e` (durable-run process-lifecycle testy, viz `tests/e2e/README.md`).

## Recipe

### Napsat integrační test handleru / repozitáře
1. `fx := testfx.New(t)` — reálná DB + migrace na adaptéru z `APP_DB_DRIVER`.
2. Naplň data: `u := fx.SeedUser(t, "bob", "secret12", "user")`.
3. Postav handler s reálnými závislostmi z `fx` (`fx.Users`, `fx.Hasher`, …).
4. Voláš handler buď přímo (eventy přes `shared.ContextWithEventCollector` + `collector.Flush()`),
   nebo přes plný chain: `cmdBus, _, _ := fx.NewBuses()` + `testfx.ExecCommand[...](...)`.
5. Asertuj přes porty (`fx.Users.FindByNickname(...)`), a kde port chybí, přes helpery (`fx.Count`, `fx.AuditEntry`, `fx.AssertTokenCount`). Do těla testu nepiš SQL; když ho test potřebuje, patří do helperu v `testfx`.

### Test repozitáře
Test portu patří do `app/internal/repotest/<ctx>/`, ne do adaptéru. Tenant id
v datech musí být skutečný tenant (`fx.SeedTenant(t, "acme").ID`), ne
libovolný řetězec: Postgres drží `uuid` typ a FK.

### Než commitnu
1. `make format` — srovná styl (ESLint Stylistic + golines).
2. `make lint` — ESLint + tsc + knip + golangci-lint + arch-check + nosqlite-check + format-check + ts-check + boundary-check + errfields-check + i18n-check + docpaths-check (+ documan lokálně).
3. `make test` — vitest + `go test`.
4. `go test -race ./app/... ./cmd/...` — **manuální** krok na souběh; **není**
   v `make test` ani v CI, ale spouští se lokálně před většími změnami.

## Invariants & pitfalls
- **testfx je test-only.** Import `app/internal/testfx` patří jen do `*_test.go`.
  Komponenta `testfx` v `.go-arch-lint.yml` smí wirovat reálnou infrastrukturu
  právě proto, že ji produkční kód nikdy neimportuje.
- **Handler testy přes bus jedou `testfx.ExecCommand`, ne `application/bus` přímo** — jinak spadne `make arch-check` (komponenta `application` nemá grant na `bus` ani na `bus_middleware`; sankcionovaná cesta je testfx).
- **Nepřejmenuj `zz_`-testy bez kontextu.** Ruší se na nich claim-ID a
  anti-vacuity kontroly; prefix `zz_` je záměrný (řazení nakonec).
- **Nový bounded context = nový `domain_<ctx>` + grant v `mayDependOn`** napříč
  konzumenty (`application`, `sqlite_repos`, `testfx`, `repotest`, …), jinak arch-check padá.
  Viz `/gk-architecture`.
- **`-race` se hlídá ručně.** Není ve `make test` ani v `validate.yml` —
  nezapomeň ho pustit u změn, co se dotýkají souběhu (collector per-request, worker).
- **Každý `testfx.New(t)` má vlastní izolovanou DB** — nesdílej fixture mezi
  testy. Na SQLite je to soubor v `t.TempDir()`.
- **V testu žádné SQL konkrétního dialektu.** `zz_nosqlite` spadne na
  `strftime(`/`julianday(`/`datetime(`/`PRAGMA`/`*.db`. Stav, který porty
  neumí, nastav helperem z `testfx` (má implementaci pro každý backend).

## Related
- Skills: `/gk-architecture` (vrstvy + go-arch-lint), `/gk-commands`, `/gk-queries`
  (struktura handlerů), `/gk-repositories` (`r.Conn(ctx)`, raw-pool výjimky), `/gk-di`.
- Docs: [Architecture](/framework/architecture) (§ go-arch-lint, cross-domain izolace).
- Kód: `app/internal/testfx/testfx.go`, `app/internal/testfx/raw.go`,
  `app/internal/repotest/`, `app/domain/zz_audit_test.go`,
  `app/domain/zz_gap_test.go`, `app/zz_nosqlite_test.go`,
  `.go-arch-lint.yml`, `.golangci.yml`, `Makefile` (`test`, `lint`, `arch-check`,
  `nosqlite-check`), `.github/workflows/validate.yml`.
- Plán: [Postgres adaptér](/framework/postgres-adapter-plan) (sekce 7).
