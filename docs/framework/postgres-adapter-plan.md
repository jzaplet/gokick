---
layout: 'page'
uri: '/framework/postgres-adapter-plan'
position: 95
slug: 'framework-postgres-adapter-plan'
parent: 'framework'
navTitle: 'Plán: Postgres adaptér'
title: 'Plán: PostgreSQL 18 adaptér'
description: 'Analýza vazeb na SQLite a plán refaktoringu na přepínatelný SQLite/PostgreSQL 18 backend — DB-agnostické testy, zámky bez globálního write-locku a multitenancy vynucená Postgresem (RLS).'
---

# Plán: PostgreSQL 18 adaptér

Stačí nastavit `APP_DB_DRIVER=postgres` v env a aplikace **i celá Go test suite** poběží na PostgreSQL 18 se stejným chováním, aniž by se kdekoli otevřela SQLite. Postgres zároveň nahradí globální write-lock zámky na úrovni řádků a přinese multitenancy vynucenou přímo databází (Row-Level Security). Tahle stránka je analýza dnešních vazeb na SQLite a fázovaný plán refaktoringu k tomu cíli.

**Status:** 🟢 plán schválen — všechna rozhodnutí D1–D10 potvrzena · fáze 0–3 hotové (fáze 3: základ Postgresu — Docker, schéma s RLS, manager se dvěma rolemi, harness a RLS sada; odchylky od plánu v sekci 9, „Fáze 3 — co dopadlo jinak") · **Roadmap:** [Roadmap (GoKick) → Škálovatelnost](/framework/gokick-roadmap) · **Analýza:** 2026-09-24

> **Konvence cest:** existující soubory cituju plnou cestou (`app/…`), aby je hlídal
> `make docpaths-check`. **Nové** balíčky, které teprve vzniknou, píšu relativně k `app/`
> (např. `infrastructure/postgres/run`).



## TL;DR

1. **Jeden přepínač, jedno místo volby.** Backend vybírá `APP_DB_DRIVER=sqlite|postgres` (plus DSN proměnné). Větví se jen `persistence.Open(cfg)` (volá ji Wire) a jeho zrcadlo v `testfx`, které čte **stejnou** proměnnou a sestaví Store **stejnou** funkcí adaptéru (`persistence.SQLiteStore`). Testy tak z principu jedou na backendu, který určuje `APP_DB_DRIVER`, se stejným zapojením jako aplikace.
2. **Dva plnohodnotné adaptéry místo `if dialect` v jednom repu.** Postgres dostane vlastní repozitáře a vlastní migrace: `timestamptz`, `boolean`, `uuid`, `bytea`, `jsonb`, `SKIP LOCKED`, RLS. Společné zůstanou doménové porty, mechanika `Conn`/tx-in-context a hlavně **jedna sada kontraktních testů**, která běží proti oběma adaptérům.
3. **Testy nezávislé na DB.**
   - `testfx.New(t)` přestane brát cestu k souboru. Týká se to 336 volání a jde o mechanický codemod.
   - 65 raw SQL literálů v testech nahradí pojmenované fixture helpery.
   - Testy, které testují jen SQLite, se na Postgresu přeskočí s uvedeným důvodem.
   - Postgres fixture je klon šablonové DB, cca 15 ms na test (změřeno).
   - **Testy běží vždy na obou DB:** `make test` je pouští paralelně a v CI jsou dva povinné joby.
4. **Trojitá pojistka „na Postgresu žádná SQLite":**
   - runtime guard: `testfx` otevře jen adaptér z `APP_DB_DRIVER` a adaptér chybějící v buildu hlasitě odmítne; testy SQLite adaptéru se při jiném driveru přeskočí (`TestMain` → `testfx.MainFor`);
   - statický gate nad testy;
   - CI job `make test-pg` builduje s `-tags nosqlite`, takže SQLite kód se vůbec nezkompiluje, a po běhu ověří, že nevznikl žádný `*.db` soubor.
5. **Zámky.** Globální single-writer nahradí MVCC a zámky na úrovni řádků.
   - Fronta runů: `FOR UPDATE SKIP LOCKED`.
   - Scheduler a migrace: advisory locky.
   - Pojistky: `lock_timeout` a `idle_in_transaction_session_timeout`.
   - Unikátnost a invarianty drží constrainty (23505 a 23503 se mapují na doménové chyby); volitelně `SERIALIZABLE` s retry.
6. **Multitenancy vynucená Postgresem.**
   - RLS (`ENABLE` + `FORCE`) na tabulkách vlastněných tenantem.
   - Tenant se předává v **tx-lokálním** GUC `app.tenant_id`.
   - Runtime role nemá `BYPASSRLS` a nevlastní tabulky.
   - Platformní a systémová rovina jede přes oddělenou login roli s `BYPASSRLS`.

   Zapomenutý `WHERE tenant_id` pak vrátí 0 řádků místo cizích dat.
7. **Doslovný port by byl chybný.** Ověřeno na reálném Postgresu (sekce 8): dnešní `ClaimDue` by na Postgresu **vydal stejný run dvěma workerům**.
8. **České řazení a UUIDv7 v obou DB.** Gridy se řadí podle české abecedy a hledání ignoruje velikost písmen i u Č/Ř/Ž, se stejným výsledkem v SQLite i v Postgresu (ověřeno na 3 085 řetězcích). Všechny primární klíče jsou UUIDv7.
9. **Existující bug už je opravený:** po startovních migracích se ztrácel limit connection poolu, a to už na SQLite (nález N1, fáze 0 ✅).


## 1. Výchozí stav — jak je SQLite zadrátovaná

### 1.1 Architektonické vazby

- **Chybí DB abstrakce.** Všude se předává konkrétní `*database.SqliteManager` (po fázi 1 `*sqlite.Manager` v `app/infrastructure/sqlite/manager.go`):
  - `BaseRepository` v `app/infrastructure/sqlite/conn.go`;
  - konstruktory pěti repozitářů;
  - `MigrationManager`;
  - tři DI providery (CommandBus, SystemCommandBus, RunWorker), kde slouží jako `shared.Transactor`. Nemají `wire.Bind`, takže interface splňuje implicitně konkrétní typ;
  - `testfx.Fixture.DB`;
  - `app.NewApplication`, který bere `*database.MigrationManager`.
- **Wire** (`app/infrastructure/di/wire_gen.go`) volá `NewSqliteManager` bezpodmínečně pro každý subcommand. `app/application.go` pouští migrace před každým subcommandem.
- **Goose** se používá přes globální API (`SetDialect("sqlite3")`, `SetBaseFS`). Kvůli tomuto globálnímu stavu fixture testy nesmí používat `t.Parallel()`.
- **Migrace:** `migrations/embed.go` embeduje plochý adresář `*.sql`. `make migrate-*` mají natvrdo `goose … sqlite3 $APP_DB_PATH`.
- **Konfigurace** zná jen `APP_DB_PATH`, `APP_DB_JOURNAL_MODE` a `APP_DB_MAX_CONNS`. Chybí volba driveru i DSN.
- **Horizontální škálování:** `serve` spouští in-process scheduler bez koordinace mezi replikami a rate-limiter drží stav v paměti procesu.

### 1.2 Inventura v číslech

| Oblast | Počet |
|---|---|
| SQL příkazy v repozitářích | 51 (46 s `?`, 5 named `:name`, 3 dynamické `IN (?,…)` buildery) |
| Časové funkce SQLite | `NowExpr` 13×, `LeaseExpr` 3×, inline `strftime` 3×, `datetime('now')` 1×, `julianday()` 12× + expression index, `CURRENT_TIMESTAMP` default 8× |
| `LIKE`, který spoléhá na ASCII case-insensitivitu SQLite | 7 |
| Boolean uložený jako INTEGER | 2 sloupce (`users.active`, `runs.cancel_requested`), dále `SUM(active)`, `cancel_requested = 1`, `int + (bool)` |
| Raw-pool zápisy mimo tx | 4 (`RecordLogin`, `RecordFailedLogin`, `ResetFailedLogin`, audit `Save`) |
| Check-then-insert toky chráněné dnes jen serializací zápisů | 4 (vytvoření uživatele, přejmenování, vytvoření tenanta, seeder) |
| Testové soubory celkem / se skutečnou DB | 134 / **69** |
| – DB přes `testfx.New*` / přes helper / přes `NewSqliteManager` | 63 / 1 / 5 |
| Volání `testfx.New*` | **336** (za běhu `go test` vznikne ≈ 390 databází) |
| Testy s vlastním raw SQL | 26 souborů, 65 literálů (47 s `?`), plus 2 přímo v `testfx.go` |
| Testy se sémantikou specifickou pro SQLite | ≈ 25 testů v 11 souborech |
| Gate testy, které skenují zdrojový kód | 9, z toho 3 vázané na SQLite nebo na cestu `infrastructure/sqlite` |
| Dokumentace zmiňující SQLite | 12 stránek (framework + README + CLAUDE.md) a 21 skillů |

Úplný seznam testových souborů je v **příloze A**. Seznam dokumentace k aktualizaci je v **příloze B**.

### 1.3 Co už je připravené

- **Porty jsou čisté:** repozitáře leží za doménovými interface. `app/domain/run/repository.go` navíc už formuluje kontrakt s ohledem na Postgres: „SQLite: writer serialization + UPDATE…RETURNING; Postgres: SELECT … FOR UPDATE SKIP LOCKED".
- **Dialektově neutrální mechaniky:** `RowsAffectedBool`, owner-fencing, `NotTerminalClause`, tx-in-context a `ContextForbidTx`.
- **Build:** `pgx` je čisté Go, takže `CGO_ENABLED=0` i statická binárka zůstávají.
- **SQLite driver při startu nic nedělá.** Jeho `init()` jen zaregistruje jméno `sqlite3`; od v0.30 jde o transpilovaný Go kód bez WASM runtime. Na Postgresu se tedy ze SQLite nic nespustí, dokud ji někdo neotevře.
- **Roadmapa** (Škálovatelnost) s Postgres adaptérem, RLS, `SKIP LOCKED` a advisory locky už počítá.


## 2. Nálezy — co by doslovný port rozbil

Seřazeno podle závažnosti. „Ověřeno" znamená, že nález byl reprodukován na reálném Postgresu (sekce 8).

| # | Nález | Dopad na Postgresu | Řešení |
|---|---|---|---|
| N1 | **Existující bug:** `MigrationManager.RunUp` volá `SetMaxOpenConns(1)` a pak `defer SetMaxOpenConns(0)`. Hodnota 0 znamená „bez limitu", takže po každém startu je pool neomezený a `MaxIdleConns` zůstane na 1. **Ověřeno:** cap 7 → 0. | Na Postgresu hrozí vyčerpání `max_connections`. Na SQLite to už dnes ruší ochranu F-047. | ✅ **Opraveno ve fázi 0:** limit se po migracích obnovuje (open i idle), s regresním testem. S goose Provider API (fáze 1) pinning na jedno spojení úplně odpadne. |
| N2 | `ClaimDue` má tvar `UPDATE … WHERE id = (SELECT … LIMIT 1)` a guard „volný nebo expirovaný" je jen v subquery. **Ověřeno:** dva workery dostanou tentýž run a druhý navíc započítá falešný `reclaims`. | Handler se spustí dvakrát (mail nebo API volání 2×), kontrakt „nejvýš jeden worker" je porušený a poison cap se falešně posouvá. | CTE s `FOR UPDATE SKIP LOCKED` a opakovaná kontrola guardu ve vnějším `WHERE`. **Ověřeno:** druhý worker neblokuje a vezme další run. |
| N3 | Check-then-insert unikátnost (`app/application/userwrite/userwrite.go`, `CreateTenant`, seeder) je dnes korektní jen díky `BEGIN IMMEDIATE`, tedy serializaci všech zápisů. | Pod READ COMMITTED projdou kontrolou obě transakce a poražená dostane 23505, tedy **500** místo 400. | Zdrojem pravdy je constraint. 23505 se namapuje podle jména constraintu na pole a vrátí `ValidationError`. |
| N4 | `DeleteIfEmptyAcrossTenants` (NOT EXISTS users/runs) může běžet souběžně s enqueue runu, a `runs.tenant_id` nemá FK. | Tenant se smaže a run zůstane bez tenanta. | FK `runs.tenant_id → tenants(id)` v PG schématu; chyba 23503 se namapuje na „tenant není prázdný". **Ověřeno:** DELETE počká na `KEY SHARE` lock souběžného INSERTu a skončí chybou 23503, run bez tenanta nevznikne. |
| N5 | `LIKE` v 7 filtrech spoléhá na ASCII case-insensitivitu SQLite (viz komentář v `app/infrastructure/sqlite/user/list.go`). **Ověřeno:** `LIKE '%ALI%'` na Postgresu nenajde `alice`. | Hledání v gridech přestane ignorovat velikost písmen. | SQLite: Unicode `LIKE` z `ext/unicode`; Postgres: `ILIKE … COLLATE app_sort`; v obou escapování `%`, `_` a `\`. **Ověřeno:** stejné výsledky vč. Č/Ř/Ž (sekce 4.2). |
| N6 | Řazení: SQLite používá `BINARY`, Postgres locale collation. `app/application/platform/query/list_pages_test.go` čeká `Default` před `acme`. | Jiné pořadí v gridech a padající test. | **České řazení v obou DB** přes stejně pojmenovanou collation `app_sort`: v SQLite `x/text/collate`, v Postgresu ICU `cs-CZ`. **Ověřeno:** identické pořadí (sekce 4.2, rozhodnutí D2). |
| N7 | NULL v `ORDER BY` (`last_login_at` v platform gridu): SQLite řadí při ASC NULL na začátek, Postgres na konec. | Jiné pořadí. | Explicitní `NULLS FIRST/LAST` v PG repozitáři. |
| N8 | Sloupce typu `uuid`. **Ověřeno:** `WHERE id = 'not-a-uuid'` na Postgresu vyhodí chybu 22P02, SQLite vrátí 0 řádků. Testy používají id, která nejsou UUID (`tnt-abc`, `does-not-exist`, `u-1`, `no-such-id` …). | Nesmyslné id z URL vrátí 500 místo 404 a testy padají. | PG repo převede neplatné UUID u lookupu na „nenalezeno" (parita se SQLite). Testová data převést na platná UUID. Audit `actor_user_id` a `target_id` nechat jako `text`. |
| N9 | Čas: `now()` v Postgresu vrací začátek **transakce**, kdežto SQLite `'now'` platí pro jeden příkaz. SQLite navíc ukládá datetime ve třech textových formátech (proto všude `julianday`). | Časy lease a zámků by uvnitř delší transakce „stály". | `statement_timestamp()` (případně `clock_timestamp()`) a typ `timestamptz`. `MsPrecisionUTC` zachovat kvůli stejným round-tripům, čtení normalizovat na UTC. |
| N10 | Dvoje hodiny: `run_at` a `expires_at` píše Go, lease a zámky počítá DB. | Když DB běží na jiném hostu, rozdíl hodin posouvá okamžik, kdy je run způsobilý ke claimu. | Zdokumentovat požadavek na NTP. Volitelně počítat `run_at` v DB (`statement_timestamp() + interval`). |
| N11 | Typy: `SUM(active)`, `int + (bool)`, `cancel_requested = 1`. | Syntaktické a typové chyby. | `COUNT(*) FILTER (WHERE active)`, `::int`, `TRUE`. |
| N12 | Audit `metadata` je `[]byte` JSON. `runs.payload` a `runs.state` jsou libovolné bajty (existuje test s NUL bajty a 2 MiB blobem). | Typ `text` odmítne bajt 0x00, `jsonb` přeformátuje `{"step":1}`. | `payload` a `state` jako `bytea`. Audit `metadata` jako `jsonb`, test porovnávat sémanticky (rovnost JSON). |
| N13 | Při souběžném startu N replik každá pouští `goose Up`. | Migrace běží souběžně. | Goose `Provider` s `lock.NewPostgresSessionLocker()` (advisory lock). |
| N14 | Scheduler běží v každé `serve` replice; `TestScheduler_TwoInstancesTickIndependently` to výslovně dokumentuje. | Každý job proběhne N× za interval. | Advisory lock pro každý job (sekce 5). |
| N15 | `sqlx` pro driver `pgx` nepřepisuje placeholdery `?` (automaticky jen named parametry). | Týká se 46 statementů a 47 testových literálů. | PG repozitáře píšou `$n` nativně, testy jdou přes helpery (sekce 7). |

**Drobnosti mimo Postgres nalezené cestou** (první dvě opraveny ve fázi 0):
- `app/domain/tenant/repository.go` má zastaralý komentář „Name is not unique", přestože unikátní index existuje.
- `docs/framework/configuration.md` uvádí default `APP_SEED_ADMIN_TENANT` jako `Tenant 1`, v kódu je `Default`.
- `GOOSE_VERSION` v Makefile je v3.27.1, v go.mod v3.27.0.


## 3. Cílová architektura

### 3.1 Balíčky

```
app/infrastructure/
  database/        driver-neutrální: Driver, tx-in-context (TxFromContext), port Migrator,
                   klasifikace DB chyb (ErrUniqueViolation{Constraint}, ErrFKViolation, ErrSerialization …)
  persistence/     Open(cfg) → *Store  ← JEDINÉ místo, kde se větví podle driveru
                   (+ build-tagged opener pro sqlite a postgres)
  sqlite/          dnešní adaptér + SqliteManager (přesun z database/), sqlite migrátor
  postgres/        nový adaptér: Manager (dva pooly, BeginTx s rovinou a GUC), BaseRepository,
    user/ token/ run/ tenant/ audit/        časové helpery, mapování chyb, advisory Locker
  seeder/          přesun z sqlite/seeder (neobsahuje SQL, je DB-neutrální)
app/internal/
  testfx/          backend-agnostické fixtures (New(t) bez cesty)
  repotest/        sdílené kontraktní testy portů (dnešní testy v infrastructure/sqlite/<ctx>)
migrations/
  sqlite/          jediný squashnutý init (20260327000001), beze změny verze
  postgres/        jeho dvojče se stejnou verzí + budoucí dvojčata (stejné verze)
```

`persistence.Store` nese doménové porty. Wire ho vezme přes `wire.FieldsOf`, takže `container_provider.go` přestane znát konkrétní repozitáře:

```go
type Store struct {
    Driver          database.Driver
    Users           user.Repository
    PlatformUsers   user.PlatformRepository
    Tokens          token.Repository
    Runs            run.Repository
    Tenants         tenant.Repository
    PlatformTenants tenant.PlatformRepository
    Audit           shared.AuditLogger
    Tx              shared.Transactor   // BeginTx / BeginReadTx / Commit / Rollback
    Locker          shared.Locker       // advisory lock; SQLite = in-process no-op (fáze 5)
    Migrator        database.Migrator
}

// Wire provider: cleanup zavře pooly (dnes se Close v produkci nevolá vůbec).
func providePersistence(cfg *config.Config, log *slog.Logger) (*persistence.Store, func(), error)
```

**Důsledky:**
- DI providery budou brát `shared.Transactor` místo `*database.SqliteManager`.
- `app.NewApplication` bude brát `database.Migrator`.
- `.go-arch-lint.yml` dostane nové komponenty `postgres_base`, `postgres_repos` a `persistence`. Hranice bounded contextů pro Postgres se vyjmenují stejně jako pro `sqlite_repos`.

### 3.2 Konfigurace

✅ **Hotovo ve fázi 3** (`app/infrastructure/config/config.go`, `loadPostgresConfig`): tři DSN jsou povinné jen s `APP_DB_DRIVER=postgres`, musí mít tvar `postgres://user@host/…` a chybová hláška DSN nikdy neopakuje (nese heslo); timeouty se parsují vždy a záporné shodí start. `APP_TEST_DB_URL` nečte config, ale testovací harness (`app/internal/testfx/pgfx`), jen z prostředí procesu.

| Env | Default | Význam |
|---|---|---|
| `APP_DB_DRIVER` | `sqlite` | `sqlite` nebo `postgres`. Striktní parsování, při neplatné hodnotě start selže (jako `getEnvBool`). |
| `APP_DB_PATH`, `APP_DB_JOURNAL_MODE` | beze změny | Jen pro SQLite. Whitelist journal módů se přesune do configu. |
| `APP_DB_URL` | — | DSN **runtime role** (tenantová rovina, podléhá RLS). Pro `postgres` povinné. |
| `APP_DB_SYSTEM_URL` | — | DSN **systémové role** (`BYPASSRLS`) pro platformní a systémovou rovinu (sekce 6). Pro `postgres` povinné. |
| `APP_DB_MIGRATE_URL` | — | DSN **vlastníka schématu**. Používá se jen pro migrace na startu, pod advisory lockem. |
| `APP_DB_MAX_CONNS` | auto | Na Postgresu platí pro každý pool zvlášť. Dimenzovat vůči `max_connections` × počtu replik. |
| `APP_DB_LOCK_TIMEOUT` / `APP_DB_STATEMENT_TIMEOUT` / `APP_DB_IDLE_TX_TIMEOUT` | `5s` / `30s` / `60s` | Session parametry Postgresu (sekce 5). `5s` odpovídá dnešnímu `busy_timeout(5000)`. |
| `APP_TEST_DB_URL` | — | Jen pro testy: admin DSN testovacího clusteru (sekce 7). |

**Driver:** `pgx/v5` přes `pgx/v5/stdlib` a `sqlx`.
- Zachová se tím interface `Conn`, `db` tagy i named queries.
- `lib/pq` ne: `[]byte` posílá jako bytea escape text, což rozbije `jsonb` sloupce.
- Nastavení poolu: `SetConnMaxLifetime` a `SetConnMaxIdleTime`, plus runtime parametry `TimeZone=UTC` a `application_name=gokick`.
- Na depguard allow-list v `.golangci.yml` přibude `github.com/jackc/pgx/v5`.

### 3.3 Migrace

- **Dva adresáře:**
  - `migrations/sqlite/` obsahuje jediný squashnutý init (`20260327000001`, sloučeno 2026-09-24). Verze se nemění, takže existující deploymenty ho přeskočí (pravidlo upgradu přes v1.4.x viz `/gk-migrations`).
  - `migrations/postgres/` obsahuje dvojče squashnutého initu se stejnou verzí `20260327000001`. SQLite historie je od 2026-09-24 sloučená do jediného initu, takže dvojče je taky jen jedno. Nové migrace se vždy píšou **ve dvojici se stejnou verzí**.
- **Goose Provider API** (`goose.NewProvider`) místo globálního stavu:
  - pro Postgres `goose.DialectPostgres` a `lock.NewPostgresSessionLocker()`;
  - pro SQLite `DialectSQLite3` bez lockeru.

  Tím odpadne i pinning na jedno spojení, a s ním bug N1.
- **Gate:** množiny verzí v obou adresářích musí být identické. Zachytí zapomenuté dvojče. ✅ `app/zz_migrations_test.go` (fáze 3).
- **`make migrate-*`** budou respektovat driver. `make migrate-create NAME=x` založí oba soubory se stejným timestampem (✅ fáze 3; `migrate-up/down/status` zatím jen SQLite). Parsování `.env` přes `grep | cut` nahradí `goose -env` nebo malý `gk` subcommand, protože DSN obsahuje `=`.
- **Role** jsou objekty clusteru, ne databáze, proto nepatří do migrací. Zakládá je init skript Postgres image (`docker/postgres/initdb/01-roles.sh`, sekce 3.4), který použije lokální DB, testovací DB i CI. Migrace dělají jen `GRANT` a RLS politiky; jména rolí jsou pevná (`gokick_app`, `gokick_system`) s možností přepsat je přes goose `ENVSUB`.


### 3.4 Lokální Postgres: Docker + OrbStack, bez portů (rozhodnutí D10 ✅)

✅ **Hotovo ve fázi 3.** Oproti náčrtu níže: služby mají profily (`db` → `postgres`, `db-test` → `test`), takže holé `docker compose up` je v SQLite projektu nespouští (příkaz, který službu jmenuje, profil zapne sám); nepotřebují `env_file` (superuser heslo je `postgres`, hesla rolí jsou výchozí hodnoty init skriptu); healthcheck ptá `pg_isready` přes **TCP**, protože během init skriptů běží dočasný server jen na socketu a `--wait` se nesmí vrátit dřív, než role existují; `db-test` má navíc `max_connections=300` (paralelní testovací balíčky). `make db-reset` = `docker compose down --volumes` nad celým projektem (zastaví i `app`/`documan`; jediný pojmenovaný volume je `pgdata`). Kontejner `app` v compose dostane DSN s hostname `db`. Docker image se v tomto prostředí stáhnout nedal, proto image, init skript a healthcheck poprvé naostro ověří CI job `postgres tests` (sekce 7.7); init skript sám je ověřený na lokálním clusteru (dvakrát po sobě, idempotentní).

**Požadavek:**
- Vývojář dál spouští jen `make build && make serve`.
- Když je v `.env` `APP_DB_DRIVER=postgres`, Postgres v Dockeru se nahodí sám.
- Žádný kontejner nepublikuje port. Port má jen gokick binárka.

**`docker/postgres/Dockerfile`** — jeden image pro vývoj, testy i CI:

```dockerfile
FROM postgres:18                 # Debian varianta: ICU (collation cs-CZ, 4.2) je součástí
COPY docker/postgres/initdb/ /docker-entrypoint-initdb.d/
HEALTHCHECK --interval=2s --timeout=3s --retries=30 \
  CMD pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"
```

`docker/postgres/initdb/01-roles.sh` se spustí jen při prvním startu, tedy nad prázdným volume:
- založí role `gokick_owner` (vlastník schématu, migrace), `gokick_app` (NOBYPASSRLS) a `gokick_system` (BYPASSRLS) s hesly z env (sekce 6);
- založí databázi `gokick`, kterou vlastní `gokick_owner`.

Je to **jediná definice rolí**: použije ji lokální DB, testovací DB i CI. Samostatný `init-roles.sql` zmíněný v 3.3 tím odpadá.

**`docker-compose.yml`** (v rootu, vedle `app` a `documan`), dvě služby ze stejného Dockerfile:

```yaml
  db:
    build: { context: ., dockerfile: ./docker/postgres/Dockerfile }
    env_file: [.env]
    labels:
      - "dev.orbstack.domains=db.${APP_DOMAIN:-gokick.local}"
    volumes:
      - pgdata:/var/lib/postgresql          # image PG 18 má data pod /var/lib/postgresql

  db-test:
    build: { context: ., dockerfile: ./docker/postgres/Dockerfile }
    env_file: [.env]
    command: ["postgres", "-c", "fsync=off", "-c", "full_page_writes=off", "-c", "synchronous_commit=off"]
    tmpfs: [/var/lib/postgresql]            # jen v RAM, po zastavení zmizí
    labels:
      - "dev.orbstack.domains=db-test.${APP_DOMAIN:-gokick.local}"
    profiles: [test]                         # holé `docker compose up` ho nespouští

volumes:
  pgdata:
```

**Jak se aplikace připojí:**
- **Nikde není `ports:`.** OrbStack směruje doménu přímo na IP kontejneru, takže `make serve` na hostu se připojí na `db.gokick.local:5432`. Je to stejná konvence jako `docs.gokick.local` u documanu a mění se jednou proměnnou `APP_DOMAIN`.
- **Služba `app` v compose** se připojí interně přes hostname `db`: `APP_DB_URL` přepíše v `environment` stejně, jako dnes přepisuje `APP_DB_PATH`.

**`.env.example`** (hesla slouží jen pro lokální vývoj; godotenv rozbalí `${APP_DOMAIN}`):

```
APP_DB_DRIVER=sqlite     # postgres → Docker služba db se nahodí sama při make build / make serve
APP_DB_URL=postgres://gokick_app:gokick_app@db.${APP_DOMAIN}:5432/gokick?sslmode=disable
APP_DB_SYSTEM_URL=postgres://gokick_system:gokick_system@db.${APP_DOMAIN}:5432/gokick?sslmode=disable
APP_DB_MIGRATE_URL=postgres://gokick_owner:gokick_owner@db.${APP_DOMAIN}:5432/gokick?sslmode=disable
```

**Makefile:**

```make
DB_DRIVER := $(shell sed -n 's/^APP_DB_DRIVER=//p' .env 2>/dev/null)

db-up:   ## Postgres nahoru, když APP_DB_DRIVER=postgres (idempotentní; pro sqlite no-op)
ifeq ($(DB_DRIVER),postgres)
	@docker compose up -d --wait db
endif

build: db-up di fe-build
	go build …
serve: db-up
	./bin/app serve
```

- **`docker compose up -d --wait db` je idempotentní.** Když kontejner běží, vrátí se za zlomek sekundy. Jinak ho (napoprvé i sestaví) spustí a počká na healthcheck. Migrace pak jako dnes pustí aplikace při startu.
- **V CI** `make build` nemá `.env`, takže driver je prázdný a `db-up` nic nedělá. CI si databázi řídí samo.
- **Pohodlné cíle navíc:**
  - `make db-down`;
  - `make db-reset` (smaže volume `pgdata`);
  - `make db-psql` = `docker compose exec db psql -U gokick_owner gokick`, tedy konzole bez portu.

**Testy a CI:**
- `make test` spustí `docker compose up -d --wait db-test` a `APP_TEST_DB_URL` sestaví z **IP kontejneru** (`docker inspect`).
- IP z hostu funguje na OrbStacku i na Linuxu, kde je bridge síť dosažitelná přímo. Port se tak nepublikuje ani v CI (7.1, 7.7).

**Předpoklad:** lokálně se používá OrbStack, stejně jako dnes pro `app` a `documan`. Docker Desktop IP kontejnerů z hostu nezpřístupňuje. Kdo ho používá, potřebuje nepovinný, necommitovaný `docker-compose.override.yml` s portem.


## 4. Postgres schéma

| Tabulka | Změny oproti SQLite |
|---|---|
| `tenants` | `id uuid`, `created_at`/`updated_at timestamptz DEFAULT now()`, unikátní index na `name` beze změny. Rovnost (UNIQUE) zůstává binární a case-sensitive jako dnes; české řazení se používá jen v `ORDER BY` (4.2). |
| `users` | `id uuid`, `tenant_id uuid REFERENCES tenants`, `active boolean DEFAULT true`, datumy jako `timestamptz`, `role` s CHECK beze změny, `nickname` UNIQUE (globálně, jako dnes), index `(tenant_id, nickname)`. |
| `refresh_tokens` | `id` a `user_id` jako `uuid`, `ON DELETE CASCADE` beze změny. **Odpadá** redundantní `idx_refresh_tokens_token_hash`, protože UNIQUE už má vlastní index. |
| `audit_log` | `id uuid`, `metadata jsonb`, `created_at timestamptz`. `actor_user_id`, `target_id` a `actor_ip` zůstávají `text` (polymorfní hodnoty; audit nikdy nesmí selhat na tvaru dat). |
| `runs` | `id uuid`, `tenant_id uuid` **nově s FK** na `tenants` (N4), `payload bytea NOT NULL`, `state bytea`, `cancel_requested boolean`, časy jako `timestamptz`. `locked_by` zůstává `text` (nejde o UUID, je to `workerID-claimUUID`). Index `idx_runs_claim ON runs (run_at) WHERE <not terminal>`. |

Z novinek Postgresu 17/18 se hodí `uuidv7()` (4.1) a `transaction_timeout` (od verze 17).

### 4.1 Identifikátory: UUIDv7 všude (rozhodnutí D1 ✅)

- **Všechny primární klíče jsou UUIDv7**, tedy časově seřazené, takže se v B-tree indexu vkládají na konec a nerozhazují stránky.
  - `tenants`, `users` a `runs` už v7 generují.
  - `refresh_tokens` a `audit_log` dnes používají v4 na třech místech: `app/domain/token/refresh_token.go`, `app/application/bus/middleware/audit.go` a `app/infrastructure/worker/run_worker_audit.go`. Tato místa se sjednotí na `uuid.NewV7()`.
  - Časová informace v id tokenu nevadí, protože tajemstvím je hash, ne id.
- **Typ sloupce:**
  - Postgres: nativní `uuid` (16 B).
  - SQLite: `TEXT`, jako dnes.
  - Generuje vždy aplikace v Go; DB default slouží jen pro ruční SQL (seedy).
- **SQL funkce `uuidv7()` existuje v obou DB:**
  - v Postgresu 18 je vestavěná;
  - v SQLite ji adaptér zaregistruje na každém spojení (jedna funkce nad `google/uuid`, žádná nová závislost). ncruces sám nabízí jen `uuid(7)`.

  Stejný SQL seed tak běží beze změny na obou (4.3).
- **Neplatné UUID z URL** vrátí v PG repozitáři „nenalezeno" (parita se SQLite, N8).

### 4.2 Řazení a vyhledávání česky — stejně v SQLite i Postgresu (rozhodnutí D2 ✅)

**Cíl:** gridy se řadí podle české abecedy (ch za h, č za c, ř za r, malá před velkými) a hledání ignoruje velikost písmen i u Č/Ř/Ž, s **identickým výsledkem v obou databázích**.

| | SQLite | Postgres |
|---|---|---|
| Řazení | collation `app_sort` registrovaná na každém spojení přes `ext/unicode` z ncruces (`RegisterCollation(conn, "cs-CZ", "app_sort")`, interně `golang.org/x/text/collate`, pravidla CLDR) | `CREATE COLLATION app_sort (provider = icu, locale = 'cs-CZ')` v migraci (ICU, pravidla CLDR) |
| Hledání bez ohledu na velikost | `ext/unicode` přepíše `LIKE` na Unicode case-insensitive (`unicode.Register(conn)`) | `ILIKE` na výrazu `… COLLATE app_sort` |
| Diakritika | rozlišuje se (`cerny` nenajde `Černý`) | rozlišuje se, stejně |
| Rovnost / UNIQUE | binární, beze změny | deterministická ICU collation = binární rovnost, beze změny |

**Pravidla implementace:**
- **Collation jen v dotazech, nikdy ve schématu ani v indexech.**
  - Repozitáře píšou `ORDER BY nickname COLLATE app_sort` v sort whitelistech.
  - Kdyby byla ve schématu, SQLite soubor by nešel otevřít v GUI nástrojích bez registrované collation (chyba „no such collation sequence").
  - Jméno `app_sort` je v obou dialektech stejné; locale je jedna konstanta v kódu (`cs-CZ`), aby šla v jiném projektu ze šablony změnit na jednom místě.
- **SQLite manager** přejde ze `sqlx.Open("sqlite3", dsn)` na `driver.Open(dsn, initFn)` + `sqlx.NewDb`, kde `initFn` na každém novém spojení zaregistruje `unicode.Register`, collation `app_sort` a `uuidv7()`. Balíček `ext/unicode` je součástí už používaného modulu ncruces, takže nepřibude žádná závislost.
- **Escapování uživatelského vstupu** (`%`, `_`, `\`) s `ESCAPE '\'` v obou dialektech. Wildcardy z vyhledávacího pole se dnes nevyescapují ani v SQLite.
- **Jde o změnu chování i pro stávající SQLite instalace:** například `acme` se nově řadí před `Default`. Test `app/application/platform/query/list_pages_test.go`, který dnes čeká binární pořadí, se upraví.
- **Ověřeno** (V18, V19):
  - 3 085 řetězců (ruční české případy + náhodné řetězce z české abecedy včetně „ch", číslic a interpunkce) je seřazeno **identicky** vzestupně i sestupně;
  - 30 vyhledávacích vzorů (Č/č, Ř/ř, Ů/ů, ß/SS, …) dává **stejné počty** shod.
- **Trvalá pojistka proti rozjetí:** CLDR data v `x/text` a v ICU se aktualizují nezávisle. Proto bude v repu uložený pevný korpus a „zlaté" očekávané pořadí a kontraktní test ho ověří v obou CI jobech. Pokud aktualizace jedné strany pořadí změní, test spadne dřív, než se to dostane k uživatelům.

### 4.3 Seedy — stejné pro oba adaptéry (rozhodnutí D8 ✅)

Adaptér se volí **při založení projektu** a data mezi SQLite a Postgresem se nepřevádějí, takže nástroj na migraci dat není potřeba. Požadavek je jen, aby **seedy dávaly stejná data na obou**:

1. **Primární cesta jsou seedy přes aplikaci:** `./bin/app seed` a případné další seed commandy přes `SystemCommandBus`, tedy repozitáře. Na obou DB jsou identické automaticky a navíc validují doménová pravidla a zapisují audit.
2. **Volitelně SQL seedy** (`INSERT INTO …`, třeba pro hromadná demo data) v **přenositelné podmnožině SQL**:
   - id jako UUID literály nebo `uuidv7()` (existuje v obou, 4.1);
   - časy jako ISO-8601 UTC literály (`'2026-01-01 00:00:00'`);
   - booleany jako `TRUE`/`FALSE` (SQLite je zná od verze 3.23);
   - žádné dialektové funkce.
3. **Kontraktní test** pustí každý SQL seed na obou backendech a porovná výsledek přes repozitáře (počty, klíčové hodnoty). Nepřenositelný seed tak spadne v CI.


## 5. Zámky a souběh

SQLite dnes serializuje **všechny** zápisy (`_txlock=immediate`, jeden writer). Je to jednoduché a korektní, ale jeden pomalý zápis zastaví celou aplikaci. Postgres dovoluje paralelní zápisy, takže každý invariant, který dnes „drží serializace", musí držet něco explicitního:

| Situace | Dnes (SQLite) | Postgres |
|---|---|---|
| Souběžné commandy | jeden po druhém (globální write-lock) | paralelně, MVCC + zámky na úrovni řádků, READ COMMITTED |
| Claim z fronty runů | serializovaný `UPDATE … WHERE id=(subquery)` | `FOR UPDATE SKIP LOCKED` a opakovaná kontrola guardu; N workerů běží paralelně bez double-claimu (ověřeno) |
| Owner-fencing (`WHERE locked_by=?`) a CAS rotace refresh tokenu | podmíněný UPDATE | beze změny. Postgres po získání zámku znovu vyhodnotí celé `WHERE` na nejnovější verzi řádku, takže je to bezpečné. |
| Brute-force čítač (`RecordFailedLogin`) | jeden `UPDATE … RETURNING` | beze změny (atomický) |
| Unikátnost nickname a jména tenanta | kontrola před zápisem (serializovaná) | rozhoduje constraint; 23505 se namapuje na pole a vrátí `ValidationError` (N3) |
| Smazání tenanta vs. souběžný insert uživatele nebo runu | serializace | FK na obou stranách (N4); FK check drží `KEY SHARE` na řádku tenanta, 23503 znamená „tenant není prázdný" |
| Invarianty přes více řádků (guardy superadmina, budoucí „poslední admin") | serializace | zámek na „kotvě": `SELECT … FROM tenants WHERE id=$1 FOR UPDATE`, případně `pg_advisory_xact_lock(key)`; alternativně `SERIALIZABLE` (viz níže) |
| Bulk operace nad mnoha řádky | serializace | zamykat v deterministickém pořadí (`… WHERE id IN (SELECT id … ORDER BY id FOR UPDATE)`); 40P01 (deadlock) je retryovatelný |
| Scheduler v N replikách | každá replika spustí job | `Locker.TryWithLock("job:<name>")`: session `pg_try_advisory_lock` na vyhrazeném spojení, job, pak `pg_advisory_unlock`. Nevlastník tick přeskočí. |
| Migrace při startu N replik | — | goose session locker (advisory lock) |
| Dlouhá transakce | freeze celé DB | drží zámky řádků a spojení a blokuje vacuum; `idle_in_transaction_session_timeout` ji ukončí. `ContextForbidTx` zůstává a mění se jen zdůvodnění. |
| Čekání na zámek | `busy_timeout(5000)` | `lock_timeout=5s` → chyba 55P03 se namapuje na retryovatelnou chybu |

**`ClaimDue` pro Postgres** (ověřený tvar):

```sql
WITH next AS (
    SELECT id FROM runs
    WHERE completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL
      AND run_at <= statement_timestamp()
      AND (locked_until IS NULL OR locked_until < statement_timestamp())
    ORDER BY run_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE runs r
SET locked_by    = $1,
    locked_until = statement_timestamp() + make_interval(secs => $2),
    reclaims     = r.reclaims + (r.locked_until IS NOT NULL)::int,
    updated_at   = statement_timestamp()
FROM next
WHERE r.id = next.id
RETURNING r.*;
```

**Volitelně `SERIALIZABLE`.** Command se přihlásí markerem (např. `shared.RequiresSerializable`). `TransactionMiddleware` ho na Postgresu otevře jako `SERIALIZABLE` a při 40001/40P01 zopakuje celý handler (nejvýš 3×, s jitterem). Bezpečné je to proto, že eventy se dispatchují až po commitu a audit leží mimo transakci. Na SQLite je marker no-op, protože SQLite serializuje vždy. Doporučuju začít bez něj a přidat ho jen tam, kde invariant nejde vyjádřit constraintem ani zámkem na kotvě.

**Volitelně `LISTEN/NOTIFY`.** Po commitu enqueue se pošle `pg_notify('gokick_runs', …)` a worker se probudí okamžitě. Polling zůstane jako fallback. Zkrátí to latenci z `APP_RUN_WORKER_POLL` na milisekundy.

**Mimo rozsah:** rate-limiter drží stav v paměti každé repliky (`app/presentation/http/middleware/ratelimit.go`), takže efektivní limit je N× nastavená hodnota. Řešení (PG tabulka s UPSERT token bucketem nebo Redis) je samostatný krok. Lockout účtu je v DB, a tedy globální už dnes.


## 6. Multitenancy vynucená Postgresem (RLS)

Dnes izoluje tenanty aplikace: `WHERE tenant_id = ?` v každém dotazu, conformance gate `zz_tenant_test.go` a fail-closed `BaseRepository.Tenant`. RLS přidá **druhou zeď v databázi**. Když se na `WHERE` zapomene, dotaz vrátí 0 řádků a zápis do cizího tenanta skončí chybou.

### 6.1 Role a roviny

```
                 ┌──────────────────────── tenantová rovina ────────────────────────┐
HTTP/bus ───────►│ pool APP_DB_URL  → role gokick_app   (NOBYPASSRLS, nevlastní tabulky)│
                 │ BEGIN; SELECT set_config('app.tenant_id', $tenant, true); …; COMMIT│
                 └───────────────────────────────────────────────────────────────────┘
                 ┌──────────────────────── systémová rovina ─────────────────────────┐
platform:*, CLI,►│ pool APP_DB_SYSTEM_URL → role gokick_system (BYPASSRLS)           │
worker claim,    │ login lookup, brute-force čítače, refresh tokeny, audit, scheduler │
scheduler        └───────────────────────────────────────────────────────────────────┘
migrace ────────► APP_DB_MIGRATE_URL → role gokick_owner (vlastník; runtime ji nepoužívá)
```

- **Dvě login role a dva pooly** místo jedné role se `SET LOCAL ROLE`. Injektované SQL v tenantové rovině se tak nemůže povýšit, protože `gokick_app` není členem `gokick_system`. Variantu se `SET LOCAL ROLE` jsem ověřil jako funkční, ale je slabší (rozhodnutí D4).
- **Rovina se určuje v ctx:** `shared.ContextWithPlane` a `shared.PlaneFrom`.
  - Nový `PlaneMiddleware` v `BaseChain` hned za Authorize nastaví rovinu `platform`, pokud `RequiredPermission()` začíná `platform:`, jinak `tenant`. Zdroj pravdy je jeden a existující gate `zz_platform_isolation_test.go` už hlídá, že se `*AcrossTenants` volá jen z `application/platform`.
  - `SystemCommandBus`, worker (claim, lease, finalize), scheduler a seeder nastaví rovinu `system`.
- **`Transactor.BeginTx(ctx)` na Postgresu** vybere pool podle roviny. V tenantové rovině navíc nastaví `set_config('app.tenant_id', tenant, true)`. Parametr `true` znamená platnost jen pro transakci, takže se hodnota nikdy nepřenese na další uživatele spojení z poolu (ověřeno).
- **Každý přístup v tenantové rovině musí běžet v transakci.** `set_config(…, true)` mimo transakci nemá žádný efekt (ověřeno). Proto:
  - `CommandBus` má `TransactionMiddleware` už dnes;
  - `QueryBus` dostane `ReadTxMiddleware` (`BEGIN READ ONLY` + scope). Na SQLite je to no-op a čte se z poolu jako dnes, bez IMMEDIATE locku;
  - cesty mimo bus (run handler bez `WithTx`, event handlery) obslouží PG `BaseRepository.Conn(ctx)`: bez transakce v ctx otevře krátkou scoped transakci pro jeden příkaz. Je to korektní, jen o tři round-tripy dražší. Metrika v logu ukáže, jestli to někde neběží na horké cestě.
- **Klasifikace markerů.** Každý z **17** dnešních markerů `tenant-scope-exempt` dostane rovinu:
  - `(rls)` znamená, že dotaz nemá `WHERE tenant_id`, ale RLS ho i tak omezí na vlastní tenant. Patří sem vlastní změna hesla a jazyka.
  - `(system)` znamená, že jde přes `r.SystemConn(ctx)`. Patří sem platformní dotazy, login lookup a brute-force čítače.
  - U `FindByID` („identity load by id") je potřeba projít volající: profil patří do tenantové roviny, refresh do systémové.

  Gate ověří, že marker `(system)` se vyskytuje jen v metodě, která volá `SystemConn`. `SystemConn` uvnitř tenantové transakce smí jen **číst**; zápis vrátí chybu, protože by tiše rozbil atomicitu.

✅ **Fáze 3:** `shared.Plane` (`PlaneTenant` je nulová hodnota, `PlanePlatform`, `PlaneSystem`), `PlaneMiddleware` v `BaseChain` před Tenant (rovina z `shared.PlaneForPermission`), `SystemPlaneMiddleware` v `SystemChain`, `QueryChain` = `BaseChain` + `ReadTxMiddleware`. `postgres.Manager.BeginTx`/`BeginReadTx` vybírá pool podle roviny a tenantovou transakci scopne přes `set_config('app.tenant_id', $1, true)`; `BeginReadTx` uvnitř otevřené transakce se k ní připojí, jen když má stejný scope (rovina a tenant); jinak vrátí chybu. Worker a scheduler rovinu v ctx **nenastavují**: jejich cross-tenant operace (claim, lease, úklid tokenů) rozhodne ve fázi 4 přímo metoda repozitáře přes `SystemConn`, a handler runu pak běží v tenantové rovině svého runu. Označit celou smyčku workeru jako systémovou by znamenalo handler z ní zase explicitně vracet do tenantové roviny — zapomenutí by bylo fail-open. `SystemConn`, markery `(rls)`/`(system)` a gate k nim patří do fáze 4 spolu s repozitáři.

### 6.2 Politiky a granty

```sql
CREATE FUNCTION gokick_current_tenant() RETURNS uuid LANGUAGE sql STABLE AS
$$ SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid $$;
-- NULLIF je nutné: po skončení tx vrací znovu použité spojení '' (ne NULL) a ''::uuid
-- by místo 0 řádků vyhodilo chybu (ověřeno).

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE  ROW LEVEL SECURITY;           -- platí i pro vlastníka
CREATE POLICY tenant_isolation ON users
  USING      (tenant_id = gokick_current_tenant())
  WITH CHECK (tenant_id = gokick_current_tenant());    -- cross-tenant INSERT/UPDATE → chyba
-- totéž pro runs; tenants: SELECT USING (id = gokick_current_tenant())
```

| Tabulka | `gokick_app` (tenantová rovina) | `gokick_system` |
|---|---|---|
| `users`, `runs` | DML, podléhá RLS | DML (BYPASSRLS) |
| `tenants` | jen `SELECT` vlastního řádku | DML |
| `refresh_tokens` | DML, podléhá RLS: token je vidět a zapisovatelný, jen když je vidět jeho uživatel (politika `EXISTS (SELECT 1 FROM users …)` běží pod politikou `users`) | DML |
| `audit_log` | **žádný grant** (a RLS bez politiky) | jen `INSERT` + `SELECT`, takže **append-only vynucuje DB** |
| `goose_db_version` | — | — (jen `gokick_owner`) |

✅ **Fáze 3** (`migrations/postgres/20260327000001_init_schema.sql`). Dvě změny oproti návrhu výše:
- **`refresh_tokens` pro tenantovou rovinu.** Odhlášení (`LogoutCommand`) maže tokeny volajícího uvnitř transakce commandu, tedy v tenantové rovině. Se „žádným grantem" by muselo psát přes `SystemConn` uvnitř tenantové transakce, což 6.1 zakazuje. Politika přes viditelnost uživatele dá tenantové rovině přesně tokeny jejích uživatelů; login, refresh a úklid expirovaných tokenů dál půjdou systémovou rovinou.
- **RLS je `ENABLE`, ne `FORCE`.** `FORCE` by politikami svázal i vlastníka schématu, takže pozdější datová migrace (`UPDATE users SET …` jako `gokick_owner`) by tiše nezměnila ani řádek. Aplikace jako vlastník nikdy neběží: kontrola rolí při startu (6.3) odmítne `APP_DB_URL`, jehož role tabulky vlastní nebo je členem jejich vlastníka. V8 (`FORCE` filtruje vlastníka) tím pádem v návrhu nevyužíváme.

### 6.3 Pojistky

- **Kontrola rolí při startu.** Superuser obchází RLS **vždy**, i s `FORCE`; typicky jde o výchozího uživatele `postgres` v dockeru. Proto se při startu ověří, že role pro `APP_DB_URL` nemá `rolsuper` ani `rolbypassrls` a nevlastní tabulky. Jinak start selže (fail fast). ✅ **Fáze 3:** `postgres.Manager.VerifyRoles`, volaný z `Migrator.RunUp` **po** migracích (teprve pak existují tabulky, jejichž vlastnictví se kontroluje). Vlastnictví zahrnuje i členství v roli vlastníka (`pg_has_role(…, 'MEMBER')`) a odmítne se i role, která je členem superusera nebo role s `BYPASSRLS` (mohla by se na ni přepnout přes `SET ROLE`). Kontroluje i systémovou rovinu: `APP_DB_SYSTEM_URL` musí mít `BYPASSRLS` (bez něj by cross-tenant čtení tiše vracelo filtrovaný výsledek) a nesmí být superuser ani vlastník.
- **RLS zapnuté i v single-tenant módu.** GUC má vždy hodnotu `DefaultTenantID`, takže se chování nemění a kód má jednu cestu. Testy tak RLS procházejí pořád.
- **Explicitní `WHERE tenant_id` v repozitářích zůstává.** Slouží jako obrana do hloubky a pomáhá planneru s indexem, a `zz_tenant` gate platí i pro PG repozitáře. RLS je záloha, ne náhrada.
- **Porušení `WITH CHECK`** (SQLSTATE 42501) je bug, protože `AssertTenantScope` měl zasáhnout dřív. Namapuje se na interní chybu a reportuje do Sentry.
- **Nová PG-only testovací sada** ověří izolaci **v databázi, nezávisle na kódu repozitářů**:
  - raw `SELECT count(*) FROM users` bez `WHERE` v transakci tenanta A vidí jen A;
  - `UPDATE` cizího řádku podle id ovlivní 0 řádků;
  - cross-tenant `INSERT` selže;
  - systémová rovina vidí vše;
  - chybějící tenant dá 0 řádků.

  ✅ **Fáze 3:** `app/infrastructure/postgres/rls_test.go` (9 testů): tenant vidí jen své řádky ve všech čtyřech tabulkách; cizí řádek podle id nejde změnit ani smazat (0 řádků); zápis do cizího tenanta (insert, přesun vlastního řádku, run, token pro cizího uživatele) skončí 42501; granty (tenantová rovina nesmí do registru tenantů ani do auditu, systémová smí do auditu jen přidávat a číst); systémová i platformní rovina vidí vše; role bez scope a neznámý tenant vidí 0 řádků; unikátnost nicku platí napříč tenanty; vlastní zápisy tenanta projdou. Mutační kontrola: po vypnutí RLS na `users` spadne 5 z 9 testů; s `set_config(…, false)` spadne test úniku tenanta přes pool.


## 7. Testy nezávislé na backendu

### 7.1 Principy

1. **Testy běží vždy proti oběma databázím** (rozhodnutí D9 ✅), lokálně i v pipeline:
   - **`make test` = SQLite + Postgres, paralelně.**
     - Nejdřív `docker compose up -d --wait db-test` (sekce 3.4): Postgres 18 na tmpfs s `fsync=off`, bez publikovaného portu. Kontejner se nechá běžet a další `make test` ho znovu použije.
     - Pak se paralelně spustí dva `go test` procesy: `APP_DB_DRIVER=sqlite` a `APP_DB_DRIVER=postgres -tags nosqlite`. Výstupy se prefixují `[sqlite]` a `[postgres]`.
     - Selhání kterékoli z nich = selhání `make test`.
   - **Bez běžícího Dockeru** `make test` **selže** se srozumitelnou hláškou a nabídne `make test-sqlite`. Druhá DB se tedy nikdy tiše nepřeskočí.
   - `make test-sqlite` a `make test-pg` zůstanou pro rychlou iteraci nad jednou DB.
   - **CI** pustí oba běhy jako dva paralelní joby a oba budou povinné checky v branch rulesetu (sekce 7.7).
   - **Čas:** dnešní Go suite na SQLite trvá 63 s včetně kompilace (V20). Postgres běh se odhaduje podobně (klon DB ≈ 15 ms × ≈ 390 fixtures, rozloženo do paralelních balíčků). Protože běží paralelně, celková doba se prodlouží jen mírně, ne na dvojnásobek.
   - Backend konkrétního běhu vybírá **stejná proměnná** jako v aplikaci, `APP_DB_DRIVER`; pro Postgres ji doplní `APP_TEST_DB_URL`, kterou `make` nastaví sám.
2. `testfx` staví Store **stejnou funkcí adaptéru jako produkce** (`persistence.SQLiteStore`, později `persistence.PostgresStore`). Od `persistence.Open` se liší jen tím, že si ponechá handle pro fixture zápisy (`app/internal/testfx/sqlite.go`, soubor s build tagem).
3. **Seedovat ručně se nic nemusí.** Harness si šablonovou DB sám vytvoří a zmigruje (7.3). Každý test si data založí fixture helpery (`SeedUser`, `SeedTenant` …) jako dnes a po testu se DB zahodí.
4. **Tělo testu neobsahuje žádné SQL specifické pro dialekt.** SQL žije buď v adaptéru, nebo v pojmenovaném fixture helperu implementovaném pro oba backendy.
5. Kontraktní testy repozitářů se napíšou **jednou** a běží **proti oběma** adaptérům.

### 7.2 Nové API `testfx`

```go
fx := testfx.New(t)              // dřív: testfx.New(t, filepath.Join(t.TempDir(), "x.db"))
fx := testfx.NewMultitenant(t)

fx.Tx                            // shared.Transactor (místo konkrétního *sqlite.Manager)
fx.Audit                         // shared.AuditLogger (místo sqliteaudit.NewRepository(fx.DB))
testfx.ActiveDriver()            // database.Driver z APP_DB_DRIVER (jen prostředí procesu)
func TestMain(m *testing.M) { testfx.MainFor(m, database.DriverSQLite) } // testy adaptéru
jwtfx.New(t, accessExp)          // JWT bez DB (gokick/app/internal/testfx/jwtfx)
```

✅ **Hotovo ve fázi 2.** `testfx.SystemCtx()` / `TenantCtx(tenantID)` se přesouvají do fáze 3: bez rovin a RLS by byly jen prázdné no-op (stejný důvod jako u `Locker` a `BeginReadTx`).

- **Codemod (✅ proveden):** 336 volání `testfx.New*` se přepíše mechanicky přes `gofmt -r 'testfx.New(a, b) -> testfx.New(a)'` a totéž pro `NewMultitenant`. Na vzorku dvou souborů ověřeno: pokryje všechny tři styly zápisu cesty. Ručně zbude jediný tabulkový případ v `app/application/platform/command/create_user_test.go` (`tc.fx(t, …)` s typem `func(*testing.T, string)`) a úklid nepoužitých importů `filepath` přes `goimports`. Jméno souboru dnes nic nenese.
- **Seed helpery** (`SeedUser`, `SeedUserInTenant`, `SeedRunInTenant` …) poběží v systémové rovině. Seedování je systémová operace a pod RLS by jinak `WITH CHECK` odmítl řádek cizího tenanta.
- **`NewJwt`** je přesunutý do samostatného balíčku bez DB, `app/internal/testfx/jwtfx` (✅). Tři testy middleware, které z `testfx` potřebují jen JWT, už nelinkují DB adaptéry.

### 7.3 Postgres harness (izolace per test)

- **Šablona:** při prvním použití v procesu vznikne databáze `gokick_tpl_<hash(migrations/postgres)>`.
  - Přípravu kryje `pg_advisory_lock` na clusteru, protože `go test` pouští balíčky jako paralelní procesy.
  - Na šablonu se pustí migrace přes goose Provider (bez globálního stavu).
  - Když se změní migrace, změní se hash, a tím i šablona.
- **Fixture:** `CREATE DATABASE gokick_t_<rand> TEMPLATE gokick_tpl_… STRATEGY FILE_COPY`, v `t.Cleanup` pak `DROP DATABASE … WITH (FORCE)`.
  - **Změřeno:** s `fsync=off`, `full_page_writes=off` a `synchronous_commit=off` trvá klon zhruba 15 ms a drop zhruba 5 ms (bez režie klienta). S `fsync=on` je to zhruba 110 ms kvůli checkpointu, proto testovací kontejner běží s vypnutými fsync flagy.
  - Celkem ≈ 390 databází × 20 ms ≈ 8 s sériově, rozložených do paralelních balíčků.
- **Role** zakládá bootstrap skript idempotentně jednou za cluster; DSN rolí sestaví harness.
- **`t.Parallel()`:** po odstranění globálního goose stavu ho lze v DB testech postupně povolit. Jde o volitelný výkonový krok.

✅ **Hotovo ve fázi 3** jako samostatný balíček `app/internal/testfx/pgfx`: `pgfx.New(t)` (klon šablony), `pgfx.NewEmpty(t)` (prázdná DB pro testy migrací), `DB.Config()` s DSN všech tří rolí. Šablona se staví pod dočasným jménem `…_build` z `template0` a přejmenuje se až po úspěšných migracích, takže přerušený běh nikdy nenechá napůl zmigrovanou šablonu. Harness role nezakládá: čeká cluster po init skriptu s výchozími hesly (`db-test`). Ověřeno i souběhem: dva testovací procesy nad studeným clusterem sdílí jednu šablonu a po sobě nenechají žádný klon. `testfx.New(t)` na Postgresu zatím hlasitě selže („no repositories yet"); fixture nad `pgfx` přibude s repozitáři ve fázi 4.

### 7.4 Kategorie testů

| Kat. | Význam | Souborů | Akce |
|---|---|---|---|
| **A** | Nezávislé na backendu, bez SQL | 26 | jen codemod `New(t)` |
| **A+** | Nezávislé na backendu, ale obsahují raw SQL nebo `fx.DB` | 14 + 1 | codemod + SQL nahradit helpery (7.5); „+1" je `app/zz_gap_test.go`, který se přepisem změní z S na A+ |
| **K** | Kontraktní testy portů (dříve v `infrastructure/sqlite/<ctx>`) | 23 | ✅ přesunuty do `app/internal/repotest/<ctx>`, poběží na obou backendech; precizní část `run/repository_test.go` se ukázala jako přenositelná (`fx.SetLeaseFromNow`) |
| **S** | Sémantika specifická pro SQLite | 5 | zůstanou v SQLite adaptéru s `TestMain` → `testfx.MainFor(m, database.DriverSQLite)` a `//go:build !nosqlite`; kde to dává smysl, vznikne **PG dvojče** |
| **G** | Gate testy nad zdrojovým kódem (bez DB) | 2 dotčené + 2 nové | sekce 7.6 |

Rozpis po souborech je v **příloze A**.

**SQLite-only testy a jejich PG dvojčata:**

| SQLite test | Co testuje | PG dvojče |
|---|---|---|
| `TestManager_ConcurrentTxWritesDoNotReturnBusy` | `_txlock=immediate` vs. `SQLITE_BUSY_SNAPSHOT` (read-modify-write bez chyby) | souběžné commandy s atomickým `val = val + 1`: nic se neztratí, žádná 500; plus test mapování 40001/55P03 |
| busy_timeout, `PRAGMA foreign_keys` pro každé spojení, whitelist journal módů | tuning SQLite DSN | `lock_timeout`, `idle_in_transaction_session_timeout` a kontrola rolí při startu (6.3) |
| `sqlite_master` introspekce, goose `Down` | schéma a rollback migrace | `pg_indexes` / `information_schema`, Down přes Provider |
| ~~raw-pool zápis čeká na write-lock~~ | ✅ ve fázi 2 se ukázal jako přenositelný: na Postgresu drží zámek řádek místo celé DB a test platí beze změny. Je v `app/internal/repotest/user/zz_audit_test.go`. | PG navíc: `lock_timeout` místo zamrznutí (fáze 5) |
| ~~precizní testy `julianday`/`strftime` (run repo)~~ | ✅ přepsány přes `fx.SetLeaseFromNow` (posun vůči hodinám DB) a jsou kontraktem v `app/internal/repotest/run/repository_test.go` | PG navíc: `statement_timestamp()` vs. `now()` v dlouhé tx |
| `sqlite_loadtest_test.go` (tag `loadtest`) | propustnost jednoho writeru | volitelně PG load test: N workerů × M runů, exactly-once, propustnost |
| — | — | **nové:** RLS sada (6.3), `SKIP LOCKED` s reálně paralelními workery, advisory lock scheduleru, souběžné migrace |

### 7.5 Raw SQL v testech → pojmenované helpery

✅ **Hotovo ve fázi 2** (`app/internal/testfx/raw.go`). Jediný dialektový kus je výraz „hodiny DB + ? sekund", který dodá backend (SQLite: `sqlite.LeaseExpr`); zbytek je přenositelné SQL s `?` přes `Rebind`.

| Dříve v testu | Helper v `testfx` |
|---|---|
| `UPDATE runs SET locked_until = strftime(…'now','-1 hour')` (`forceExpire`, `forceExpireW`) | `fx.ForceExpireLease(t, runID)` |
| `UPDATE runs SET locked_by=?, locked_until=strftime(…'+1 hour')` (`stealLeaseW`) | `fx.StealLease(t, runID, owner)` |
| `UPDATE runs SET run_at = strftime(…'-1 second')` | `fx.MakeRunDue(t, runID)` |
| `UPDATE runs SET completed_at/failed_at = strftime(…)` | `fx.ForceRunCompleted(t, runID)` / `fx.ForceRunFailed(t, runID)` |
| `UPDATE runs SET locked_until = julianday(…) ± ms` | `fx.SetLeaseFromNow(t, runID, d)` |
| `UPDATE runs SET reclaims/parks = ?` | `fx.SetRunReclaims(t, runID, n)` / `fx.SetRunParks(t, runID, n)` |
| `UPDATE users SET active = 0 WHERE id = ?` (5×) | `fx.SetUserActive(t, userID, false)` |
| `UPDATE users SET locked_until = ?` | `fx.SetUserLockedUntil(t, userID, ts)` |
| `INSERT INTO users … datetime('now') …`, `INSERT INTO refresh_tokens …` (testy constraintů) | `fx.RawExec(query, args…)` s přenositelným SQL a assert přes **klasifikovanou** chybu `fx.Violated(err)` → `testfx.NotNull` / `Unique` / `Check` / `ForeignKey` (z kódu chyby driveru, ne z textu — Postgres píše „not-null") |
| `SELECT COUNT(*) FROM audit_log/runs/users/tenants …` | `fx.Count(t, table, where, args…)` |
| `SELECT … FROM audit_log WHERE id=?` | `fx.AuditEntry(t, id)` |
| `UPDATE tenants SET plan=?` (`SeedTenantWithPlan`) | beze změny API, interně přes fixture handle |
| `sqliteaudit.NewRepository(fx.DB)`, `provideCommandBus(…, fx.DB, …)` | `fx.Audit`, `fx.Tx` |

**Úpravy testových dat:**
- ✅ Id, která nejsou UUID (`tnt-abc`, `tnt-123`, `tnt-7`, `T1/T2`, `tenant-A`, `tenant-x`, `u-1/u-2`), jsou nahrazená. Tenanti v datech runů jsou skuteční (`fx.SeedTenant(t, "acme").ID`), protože Postgres vedle typu `uuid` drží i FK `runs.tenant_id`.
- „Neexistující" id (`does-not-exist`, `no-such-id`) mohou zůstat, protože PG repo neplatné UUID vrátí jako „nenalezeno" (N8).
- ✅ Porovnání audit `metadata` je sémantická rovnost JSON.

### 7.6 Gate testy

| Gate | Změna |
|---|---|
| `app/infrastructure/sqlite/zz_tenant_test.go` | Skener se parametrizuje adresářem a poběží pro `infrastructure/sqlite` i `infrastructure/postgres` (fáze 4). **Guard proti prázdnému výsledku** ✅ (fáze 2): každý balíček repozitáře musí dát aspoň jeden dotaz a write gate aspoň jeden INSERT s `tenant_id`, jinak test selže jako „scan went blind". Pro PG SQL se opraví `tableRe`, který by jinak chytal `FOR UPDATE SKIP LOCKED` jako tabulku `skip`, podobně `ON CONFLICT … DO UPDATE SET`, `EXTRACT(… FROM …)` a `FROM unnest(…)`. Exempt tabulky budou pro každý dialekt zvlášť. Markery dostanou rovinu `(rls)` nebo `(system)` (6.1). |
| `app/infrastructure/sqlite/zz_sqltime_test.go` | Zůstane jen pro SQLite; guard proti prázdnému výsledku ✅ (fáze 2). PG dvojče zakáže `now()` a `CURRENT_TIMESTAMP` v repo SQL a vyžádá `statement_timestamp()`/`clock_timestamp()` (N9). |
| `app/domain/zz_gap_test.go` | ✅ Mezi zakázanými importy handlerů jsou všechny DB kořeny: `infrastructure/sqlite`, `infrastructure/postgres`, `infrastructure/persistence`. |
| `app/infrastructure/worker/zz_notx_test.go` | Beze změny, kromě komentáře. |
| ✅ **nový** `app/zz_nosqlite_test.go` | Soubor, který importuje SQLite adaptér nebo ncruces, musí mít `//go:build !nosqlite` (mimo adaptér jsou to jen dva openery). Každý soubor pod `infrastructure/sqlite/**` ten tag má a každý jeho testovací balíček má `TestMain` s `testfx.MainFor(m, database.DriverSQLite)`. Testy a fixtures mimo tagované soubory nenesou SQLite dialekt: `julianday(`, `strftime(`, `datetime(`, `PRAGMA`, `sqlite_master`, `INSERT OR …`, cestu `*.db`. Každé pravidlo má vlastní „bite" test. |
| ✅ **nový** `app/zz_migrations_test.go` (fáze 3) | Množiny verzí v `migrations/sqlite/` a `migrations/postgres/` se musí shodovat; soubor bez verze i prázdný adresář gate shodí, „bite" test pokrývá obě strany. |

### 7.7 Trojitá pojistka „žádná SQLite na Postgresu"

1. **Runtime:** `testfx` otevře jen adaptér z `APP_DB_DRIVER` a testy SQLite adaptéru se při jiném driveru přeskočí (`TestMain` → `MainFor`). V buildu bez adaptéru (`-tags nosqlite`) selže fixture i `persistence.Open` nahlas („not built into this binary"), místo aby cokoli tiše běželo na SQLite. ✅
2. **Staticky:** gate `zz_nosqlite` (7.6) a `go-arch-lint`: jen `persistence` a `infrastructure/sqlite/**` smí záviset na SQLite adaptéru.
3. **Kompilace a artefakty v CI:**
   - `make test-pg` = `go test -tags nosqlite ./...` s `APP_DB_DRIVER=postgres`. SQLite adaptér, SQLite-only testy i registrace ncruces driveru jsou za `//go:build !nosqlite`, takže jakákoli zbylá závislost **neprojde kompilací**.
   - Dále `go list -tags nosqlite -deps ./... | grep ncruces` musí být prázdné.
   - ✅ Už od fáze 2 to hlídá `make nosqlite-check` (součást `make lint`): `golangci-lint --build-tags nosqlite` nad celým stromem včetně testů a `go list -tags nosqlite -test -deps`, který nesmí obsahovat ncruces ani adaptér.
   - Test proběhne s `TMPDIR` v čerstvém adresáři a po běhu tam nesmí být žádný `*.db`, `*-wal` ani `*-journal`.

**CI** (`.github/workflows/validate.yml`), rozhodnutí D7:
- **Dva paralelní joby, oba povinné:**
  - `validate` zůstává jako dnes: lint, testy na SQLite, build;
  - nový `test-postgres` spustí `docker compose up -d --wait db-test`, tedy **stejný** `docker/postgres/Dockerfile` a init skript jako lokálně. `APP_TEST_DB_URL` sestaví z IP kontejneru (na Linux runneru je dosažitelná přímo, bez publikovaného portu) a pak pustí `go test -tags nosqlite` s `APP_DB_DRIVER=postgres`.

  Protože joby běží souběžně, pipeline se neprodlouží. Lint poběží pro obě sady build tagů.

  ✅ **Částečně už ve fázi 3:** job `postgres tests` (`make test-pg`: `db-test` ze stejného Dockerfile, IP kontejneru, `-tags nosqlite`) běží od fáze 3. Zatím pokrývá jen balíčky Postgres adaptéru a ještě není povinný; celá suite a povinnost v rulesetu zůstávají ve fázi 6.
- **Kde to běží a kolik to stojí:**
  - **GitHub-hosted runnery.** `jzaplet/gokick` je veřejný repozitář, takže minuty jsou **zdarma** a Postgres se startuje v rámci jobu; žádný vlastní server není potřeba.
  - Projekty založené ze šablony jako **privátní** repozitáře čerpají měsíční kvótu minut z GitHub plánu. Jeden běh je řádově 2 × ~1–2 min, takže běžný provoz se vejde; aktuální kvóty a ceny je potřeba ověřit v ceníku GitHubu.
  - **Self-hosted runner** na vlastním dedikovaném serveru je volitelná optimalizace **jen pro privátní repozitáře**: teplá cache a vlastní CPU = rychlejší běh. Na veřejném repu je to bezpečnostní riziko, protože PR z forku by mohl spustit cizí kód na serveru; GitHub to u veřejných repozitářů nedoporučuje.
- **E2E:** `tests/e2e/lib.sh` dostane PG variantu. `at_least_once.sh` dnes volá CLI `sqlite3`; nahradí ho `psql` nebo počet přes `/debug/runs`. Odložený e2e test *multi-process fencing* (dva procesy workeru nad jednou DB) má na Postgresu konečně smysl.


## 8. Ověřeno na reálném Postgresu

Tvrzení v nálezech a v návrhu jsem ověřoval na lokálním PostgreSQL 16.13; image verze 18 v tomto prostředí nebyl k dispozici. Použité mechanismy (row locks, `SKIP LOCKED`, RLS, GUC, advisory locky, `CREATE DATABASE … TEMPLATE … STRATEGY`) se ve verzi 18 chovají stejně. PG18-specifické je jen `uuidv7()` a nová cesta `PGDATA` v oficiálním docker image, proto je potřeba mountovat `/var/lib/postgresql`.

| # | Experiment | Výsledek |
|---|---|---|
| V1 | Doslovný port `ClaimDue`, dvě souběžné session | ❌ **oba workery dostaly tentýž run**, druhý navíc `reclaims=1` |
| V2 | `ClaimDue` s CTE `FOR UPDATE SKIP LOCKED` | ✅ druhý worker neblokuje a vezme další run, `reclaims=0` |
| V3 | RLS, runtime role bez nastaveného tenanta | ✅ 0 řádků (fail-closed) |
| V4 | `set_config(…, true)` v tx, pak stejné spojení po `COMMIT` | ✅ tenant platí jen uvnitř tx, po commitu 0 řádků (nic se nepřenese přes pool) |
| V5 | `set_config(…, true)` v autocommitu | ✅ bez efektu, takže čtení v tenantové rovině musí běžet v tx |
| V6 | Cross-tenant `INSERT` a `UPDATE` podle id | ✅ `INSERT` selže na RLS policy, `UPDATE` ovlivní 0 řádků |
| V7 | Role s `BYPASSRLS` / vlastník s `FORCE` | ✅ systémová role vidí vše, vlastník s `FORCE` je filtrovaný |
| V8 | Globální `UNIQUE(nickname)` pod RLS | ✅ vynuceno napříč tenanty (23505) |
| V9 | Politika bez `NULLIF`, znovu použité spojení | ⚠️ `''::uuid` vyhodí chybu, proto je `NULLIF` nutné |
| V10 | `WHERE id = 'not-a-uuid'` nad `uuid` | ⚠️ chyba 22P02, proto parse v repu (N8) |
| V11 | `LIKE` vs. `ILIKE` | ⚠️ `LIKE` je case-sensitive (N5) |
| V12 | `SET LOCAL ROLE` na `BYPASSRLS` roli (alternativa D4) | ✅ funguje, po commitu se vrací původní role |
| V13 | `lock_timeout=1s` proti drženému zámku řádku | ✅ „canceling statement due to lock timeout" (55P03) |
| V14 | `pg_try_advisory_lock`, dvě session | ✅ zámek získá jen první (leader election pro scheduler) |
| V15 | DELETE tenanta vs. souběžný INSERT runu s FK | ✅ DELETE čeká ≈ 1,5 s na `KEY SHARE` lock a skončí chybou 23503, run bez tenanta nevznikne |
| V16 | `CREATE DATABASE … TEMPLATE` × 30 | ✅ `FILE_COPY` + `fsync=off` ≈ 15 ms klon, ≈ 5 ms drop (bez režie klienta) |
| V17 | Pool cap po `RunUp` (dnešní SQLite kód) | ❌ **7 → 0 (bez limitu)**, bug N1 |
| V18 | České řazení: SQLite (ncruces `ext/unicode`, `x/text/collate` `cs-CZ`, registrace na spojení) vs. Postgres (ICU 74 `cs-CZ`), 3 085 řetězců | ✅ **identické pořadí** ASC i DESC (ch za h, č za c, malá před velkými) |
| V19 | Vyhledávání bez ohledu na velikost: SQLite Unicode `LIKE` vs. Postgres `ILIKE … COLLATE`, 30 vzorů | ✅ **identické počty** shod (Č/č, Ř/ř, Ů/ů, ß/SS; diakritika rozlišená v obou). Bez `unicode.Register` SQLite ignoruje velikost jen u ASCII. |
| V20 | Doba dnešní Go suite na SQLite (`go test ./app/... ./cmd/...`) | 63 s včetně kompilace, 40 balíčků OK |


## 9. Plán refaktoringu — fáze a PR

Pořadí je zvolené tak, aby **fáze 1 a 2 byly čisté refaktory bez změny chování**, zelené na SQLite. Když pak přibude Postgres, celá suite už je na backendu nezávislá. Každá fáze = jeden nebo více PR s Conventional Commits (`refactor:`, `test:`, `feat(db):` …).

| Fáze | Obsah | Hotovo, když |
|---|---|---|
| **0 — bugfix** ✅ | Oprava N1 (pool cap po migracích) a regresní test. Drobnosti z konce sekce 2. | ✅ hotovo: `MaxOpenConnections` po `RunUp` = nastavený cap, idle limit taky |
| **1 — švy (jen SQLite)** ✅ | Migrace v `migrations/sqlite/` přes goose Provider API (bez globálního stavu; Provider sám drží jedno `*sql.Conn`, takže pinning poolu odpadl). `database` je driver-neutrální (tx v kontextu, port `Migrator`); `sqlite.Manager` a `sqlite.Migrator` v adaptéru. `persistence.Store` + `Open`, Wire přes `FieldsOf`, cleanup zavírá pool při ukončení. Providery berou `shared.Transactor`, `NewApplication` bere `Migrator`. Seeder v `infrastructure/seeder`. `.go-arch-lint.yml` upraven. Porty `shared.Locker` a `Transactor.BeginReadTx` přesunuty do fáze 3 (bez PG by byly jen prázdné no-op). | ✅ hotovo: lint, arch-check, testy a gates zelené; ověřeno na binárce (migrace, seed, serve, login) |
| **1b — české řazení a UUIDv7 (jen SQLite, `feat`)** ✅ | SQLite manager přes `driver.Open` + init callback `registerConnFuncs`: Unicode `LIKE`, collation `app_sort` (`cs-CZ`), SQL funkce `uuidv7()`. Sort whitelisty a textové tie-breaky s `sqlite.CollateSort`, filtry přes `sqlite.LikeContains` + `LikeEscape`. `refresh_tokens` a `audit_log` id na v7. Zlatý korpus `app/infrastructure/database/testdata/sort_cs/` (očekávání vyrobil Postgres, ICU `cs-CZ`) + test `app/infrastructure/sqlite/collation_test.go`. Úprava `list_pages_test`. | ✅ hotovo: zlatý test zelený (a spadne s binární collation), celá suite zelená |
| **2 — testy nezávislé na backendu (ještě SQLite)** ✅ | `APP_DB_DRIVER` + `database.Driver`; `persistence.Open` větví podle driveru a SQLite opener je soubor s tagem `!nosqlite` (celý adaptér taky). Codemod `testfx.New(t)`; `fx.Tx`, `fx.Audit`, helpery ze 7.5; UUID / skuteční tenanti v testových datech. Kontraktní testy v `app/internal/repotest/<ctx>` (vč. testů, které se ukázaly jako přenositelné: raw-pool vs. zámek, ms přesnost leasu, no-tx zóna, kaskáda tokenů); testy adaptéru s `TestMain` → `testfx.MainFor`. Gate `app/zz_nosqlite_test.go`, guard proti prázdnému výsledku v `zz_tenant`/`zz_sqltime`, handler gate na všechny DB kořeny; `make nosqlite-check` v `make lint`. `NewJwt` → `jwtfx`. `SystemCtx`/`TenantCtx` přesunuty do fáze 3. | ✅ hotovo: mimo `infrastructure/sqlite/**` a tagované openery není v testech ani řádek SQLite SQL; s `-tags nosqlite` se celý strom včetně testů zkompiluje a lintuje bez ncruces; test pak hlásí „no fixture backend for APP_DB_DRIVER=postgres" |
| **3 — PG základ** ✅ | Config (sekce 3.2), `pgx`, `postgres.Manager` se dvěma pooly, rovinami a GUC, kontrola rolí při startu (`VerifyRoles`), session timeouty. Port `Transactor.BeginReadTx` (SQLite: no-op). `shared.Plane`, `PlaneMiddleware`, `SystemPlaneMiddleware`, `ReadTxMiddleware` v `QueryChain` (na SQLite beze změny chování). `migrations/postgres/` init s typy, FK, RLS a granty; embed po dialektech (SQLite sada jen v buildu bez `nosqlite`); gate dvojčat `app/zz_migrations_test.go`; `make migrate-create` zakládá obě dvojčata. `postgres.Migrator` jako vlastník, pod advisory lockem. `docker/postgres/Dockerfile` + `initdb/01-roles.sh`, služby `db` a `db-test` (OrbStack domény, bez portů), `make db-up` v `build`/`serve` + `db-down`/`db-reset`/`db-psql`, `.env.example`. Harness `app/internal/testfx/pgfx`, `testfx.SystemCtx()` pro seed helpery, `make test-pg`, CI job `postgres tests`. | ✅ hotovo: migrace, harness a RLS sada zelené na PostgreSQL 16 lokálně (PG 18 image ověří CI job); celá suite na SQLite beze změny; `persistence.Open` Postgres zatím odmítá („not available yet"), dokud nejsou repozitáře |
| **4 — PG repozitáře** | `user`, `tenant`, `token`, `run`, `audit` v `$n` SQL; `ClaimDue` se `SKIP LOCKED`; časy přes `statement_timestamp()`; `ILIKE` s escapováním; `NULLS FIRST/LAST`; parse UUID; mapování chyb (23505, 23503, 42501, 40001, 40P01, 55P03, 22P02). `zz_tenant` pro PG. | **Celá suite zelená na obou backendech**; `make test-pg` s `-tags nosqlite` |
| **5 — zámky** | Advisory `Locker` pro scheduler, locker migrací, timeouty (sekce 3.2), zamykání „kotvy" u invariantů, deterministické pořadí u bulk operací, volitelný `RequiresSerializable` + retry. PG testy souběhu (N workerů × M runů exactly-once, dvě instance scheduleru, souběžné migrace). Úprava `TestScheduler_TwoInstancesTickIndependently`. | Testy souběhu zelené na PG a opakovaně (`-count=20`) |
| **6 — CI a pojistky** | Paralelní joby `validate` (SQLite) a `test-postgres` (stejná služba `db-test` jako lokálně, IP kontejneru místo portu) v `validate.yml`; `make test` = obě DB paralelně (služba `db-test`, bez Dockeru selže s nabídkou `make test-sqlite`). Lint pro obě sady tagů, kontrola artefaktů `*.db` a `go list -deps`, e2e na PG + multi-process fencing. | Oba joby povinné v branch rulesetu; `make test` pouští obě DB |
| **7 — dokumentace** | Příloha B: CLAUDE.md, skilly, framework docs, README, roadmapa (zapsat rozhodnutí D1–D9). | `make docpaths-check` + `documan-lint` zelené |
| volitelně | `LISTEN/NOTIFY` wake-up workeru; produkční build `-tags nosqlite`; sdílený stav rate-limiteru; `t.Parallel()` v DB testech; self-hosted runner pro privátní projekty ze šablony. | — |


### Fáze 3 — co dopadlo jinak

- **Posunuto dopředu:** session timeouty (`lock_timeout`, `statement_timeout`, `idle_in_transaction_session_timeout`) a advisory lock migrací (goose session locker, zámek se zkouší každou sekundu až 5 minut; výchozích 5 s by každou čekající repliku zdrželo) byly v plánu ve fázi 5. Jsou to jen parametry spojení a jedna volba Provideru, takže přišly rovnou s managerem. Test: `TestMigrator_ConcurrentRunUp`. Stejně tak CI job `postgres tests` (jinak fáze 6), protože Docker image se dal ověřit jedině v CI.
- **Posunuto dál:** port `shared.Locker` (SQLite: in-process no-op) přijde až ve fázi 5 spolu se schedulerem, který je jeho jediným spotřebitelem; bez něj by to byl mrtvý kód. `testfx.TenantCtx` taky počká na testy, které ho budou potřebovat (fáze 4); `testfx.SystemCtx()` už seed helpery používají.
- **Schéma a granty:** `refresh_tokens` je pro tenantovou rovinu přístupné přes RLS místo „žádného grantu" a RLS je `ENABLE`, ne `FORCE`. Důvody jsou v sekci 6.2.
- **Roviny workeru a scheduleru** se neoznačují v ctx, rozhodne o nich metoda repozitáře (sekce 6.1).
- **Adaptér zatím není volitelný:** `persistence.Open` pro `postgres` vrací „not available yet" a `make build && make serve` s `APP_DB_DRIVER=postgres` sice nahodí databázi, ale aplikace start odmítne. `make test-pg` pokrývá balíčky adaptéru (`PG_TEST_PKGS`); ve fázi 4 se rozšíří na celou suite.
- **`make migrate-up/down/status`** dál obsluhují jen SQLite soubor. Na Postgresu migruje aplikace při startu; ruční cesta (plán v 3.3: `goose -env` nebo `gk` subcommand) zůstává na později.
- **Ověřeno:** PostgreSQL 16 lokálně (image verze 18 se v tomto prostředí stáhnout nedal). Schéma nepoužívá nic, co PG 16 nemá (`uuidv7()` generuje aplikace; ve schématu není jako DEFAULT).

## 10. Rozhodnutí

Stav k 2026-09-24: všechna rozhodnutí potvrzena ✅.

| # | Otázka | Stav | Rozhodnutí / doporučení |
|---|---|---|---|
| D1 | Identifikátory | ✅ | **UUIDv7 všude.** V Postgresu typ `uuid`, v SQLite `TEXT`; `uuidv7()` jako SQL funkce v obou (4.1). |
| D2 | Řazení | ✅ | **Česky, identicky v obou DB.** Collation `app_sort` = `cs-CZ` (4.2, ověřeno V18/V19). |
| D3 | Fronta na background práci: vlastní engine, nebo River | ✅ | **Vlastní engine + `SKIP LOCKED`**, viz vysvětlení níže. |
| D4 | Jak se k datům dostane „systémová" část aplikace | ✅ | **Dvě DB role (dva účty)**, viz níže. |
| D5 | RLS i v režimu bez multitenancy | ✅ | **Zapnuté**, viz níže. |
| D6 | Výchozí driver | ✅ | **`sqlite`** |
| D7 | Kde běží Postgres pro testy | ✅ | **Lokálně Docker (`make test`), v CI GitHub-hosted runner** — pro veřejný repo zdarma, bez vlastního serveru (7.7). Self-hosted runner jen volitelně pro privátní projekty. |
| D8 | Převod dat mezi DB | ✅ | **Nepotřeba** — adaptér se volí při založení projektu; seedy musí dávat stejná data na obou (4.3). |
| D9 | Testy na obou DB | ✅ | **Vždy obě:** `make test` pouští obě paralelně, v CI dva povinné joby (7.1, 7.7). |
| D10 | Jak se spouští Postgres | ✅ | **`docker/postgres/Dockerfile` + služby `db`/`db-test` v `docker-compose.yml`, OrbStack domény, žádné porty; `make build`/`make serve` DB nahodí samy** (3.4). |

### D3 — vlastní fronta, nebo River

„Fronta" je engine pro práci na pozadí, například odeslání mailu, webhook, velký import nebo generování reportu (`/gk-runs`). gokick má vlastní: tabulka `runs` + worker. **River** je hotová open-source Go knihovna na fronty, postavená **výhradně na Postgresu**.

| | Vlastní engine (+ `SKIP LOCKED`) | River |
|---|---|---|
| Běží na SQLite i Postgresu | ✅ stejné chování, jedna sada testů | ❌ jen Postgres, pro SQLite by zůstal vlastní engine, tedy dvě různé fronty |
| Práce na přechodu | malá: přepsat `ClaimDue` + časové funkce | velká: nové tabulky, nové API workerů, přepis handlerů a testů |
| Funkce gokicku (checkpoint + resume dlouhých běhů, owner fencing, dvoufázový cancel, oddělené čítače retry/reclaim/park, propagace tenanta a jazyka, zákaz transakcí v handleru) | ✅ hotové a otestované | část má jinou podobu nebo chybí, musely by se dopsat nad River |
| Hotové „navíc" (webové UI fronty, priority, unikátní joby, periodické joby, dávky) | ❌ když budou potřeba, dopíšou se | ✅ |
| Kdo to udržuje | my | komunita / autoři Riveru |

**Rozhodnuto: vlastní engine.** River by dával smysl až ve chvíli, kdy by se SQLite úplně opustila, nebo když bude potřeba jeho UI či pokročilé funkce. Roadmapu (bod „Durable fronta na Postgresu → River") pak upravit.

### D4 — dvě DB role, nebo jedna s přepínáním

RLS znamená, že databáze sama pustí aplikaci jen k řádkům aktivního tenanta. Některé části aplikace ale **musí vidět všechny tenanty**: superadmin, CLI příkazy, worker, který vybírá runy napříč tenanty, a login, protože před přihlášením ještě tenanta neznáme. Tyto části potřebují DB účet, který RLS smí obejít.

| | A — dva DB účty (dvě DSN, dva pooly) | B — jeden účet, který se v transakci přepne (`SET LOCAL ROLE`) |
|---|---|---|
| Bezpečnost | běžný účet obejít RLS **fyzicky nemůže**; i kdyby se někdy objevila SQL injection v běžné části, cizí tenanty neuvidí | kdo dokáže přes aplikaci spustit libovolné SQL, může se sám přepnout a RLS obejít |
| Konfigurace | o jednu proměnnou a heslo víc (`APP_DB_SYSTEM_URL`) | jedna DSN |
| Spojení na DB | dva pooly, tedy o něco víc spojení | jeden pool |
| Ověřeno | — | V12 funguje |

**Rozhodnuto: A (dva účty).** Cena je jedna proměnná navíc, přínos je, že izolaci tenantů nejde obejít ani chybou v aplikaci.

### D5 — RLS i bez multitenancy

gokick má ve výchozím stavu multitenancy vypnutou (`APP_MULTITENANCY=false`). Všechna data pak patří jednomu tenantovi „Default" a uživatel žádné tenanty nevidí. Otázka zní, jestli má Postgres RLS kontrolovat i v tomto režimu.

- **Zapnuté** (rozhodnuto):
  - aplikace do DB vždy pošle tenanta „Default", takže uživatel nepozná žádný rozdíl;
  - běží jedna a tatáž cesta kódu v obou režimech;
  - testy ověřují RLS pořád;
  - pozdější zapnutí multitenancy už nevyžaduje žádnou změnu v databázi.

  Režie je zanedbatelná (jedno porovnání UUID na řádek).
- **Vypnuté:** o chlup jednodušší DB, ale vzniknou dvě cesty kódu a zapnutí multitenancy později znamená zapínat RLS na živé databázi.


## 11. Rizika

- **Rozsah.** Fáze 2 sahá do 69 testových souborů. Codemod je mechanický, ale review je velké. Proto je v samostatném PR bez změny chování.
- **Dvě sady SQL.** Každý nový dotaz vznikne dvakrát. Mitigace: sdílené kontraktní testy (drift okamžitě spadne na jednom z backendů), gate na dvojčata migrací a parametrizovaný `zz_tenant`.
- **RLS a testy s `context.Background()`.** Testy, které volají repozitář bez tenanta pro jiný než výchozí tenant, spadnou na Postgresu na `WITH CHECK`. Je to záměr, ale očekávám desítky úprav: `TenantCtx`/`SystemCtx`.
- **Výkon scoped mini-transakcí** mimo bus (tři round-tripy navíc). Mitigace: bus cesty mají transakci na požadavek, zbytek hlídá metrika.
- **Rozdíl hodin aplikace a DB** (N10) při odděleném DB hostu.
- **České řazení mění pořadí i na stávajících SQLite instalacích** (fáze 1b). Je to záměr, ale je potřeba to uvést v changelogu.
- **Rozjetí CLDR dat** mezi `x/text` (SQLite) a ICU (Postgres) po aktualizaci jedné ze stran. Mitigace: zlatý korpus řazení v kontraktních testech obou jobů (4.2).
- **Superuser v DSN** tiše vypne RLS. Proto je kontrola rolí při startu povinná a ne jen doporučená.


## Příloha A — všechny testové soubory se skutečnou DB (69)

Inventura je stav **před fází 2**; cesty v tabulce jsou aktuální (kontraktní testy už žijí v `app/internal/repotest/`). Co fáze 2 udělala jinak, než inventura předpokládala, je pod tabulkou.

Kategorie: **A** nezávislý na backendu · **A+** nezávislý, ale s raw SQL nebo `fx.DB` · **K** kontraktní test portu → `internal/repotest` · **S** jen SQLite (+ PG dvojče) · *fx* = počet volání `testfx.New*`.

| Soubor | fx | Vazba na SQLite | Kat. |
|---|---|---|---|
| `app/application/auth/command/login_test.go` | 15 | `UPDATE users SET active = 0 WHERE id = ?`; komentář o write-locku v deadlock testu | A+ |
| `app/application/auth/command/logout_test.go` | 4 | — | A |
| `app/application/auth/command/refresh_token_test.go` | 12 | `active = 0`; `"database is locked"` jen jako fake chyba (neškodí) | A+ |
| `app/application/auth/command/zz_audit_test.go` | 6 | — (UTC normalizace je obecná) | A |
| `app/application/auth/command/zz_gap_test.go` | 3 | — | A |
| `app/application/bus/middleware/zz_audit_test.go` | 2 | import `sqlite/audit`, `TransactionMiddleware(fx.DB)`, 3× `COUNT … ?` | A+ |
| `app/application/dashboard/query/get_admin_dashboard_test.go` | 1 | `active = 0` | A+ |
| `app/application/platform/command/bulk_users_test.go` | 3 | — | A |
| `app/application/platform/command/create_tenant_test.go` | 7 | — | A |
| `app/application/platform/command/create_user_test.go` | 7 | — | A |
| `app/application/platform/command/platform_test.go` | 12 | — | A |
| `app/application/platform/command/tenant_test.go` | 12 | — (`SeedTenantWithPlan` řeší testfx) | A |
| `app/application/platform/query/get_tenant_test.go` | 1 | id `no-such-id` (N8, řeší repo) | A |
| `app/application/platform/query/list_pages_test.go` | 2 | `UPDATE tenants SET plan … ?`; očekává BINARY řazení (N6) | A+ |
| `app/application/platform/query/platform_test.go` | 2 | — | A |
| `app/application/profile/command/change_password_test.go` | 6 | — | A |
| `app/application/profile/command/zz_audit_test.go` | 1 | — | A |
| `app/application/profile/query/get_profile_test.go` | 3 | — | A |
| `app/application/run/dispatcher_test.go` | 6 | tenant `tenant-x` → UUID | A |
| `app/application/user/command/bulk_users_test.go` | 6 | `active = 0` | A+ |
| `app/application/user/command/create_user_test.go` | 10 | `SELECT COUNT(*) … nickname = 'alice'` přes `fx.DB` | A+ |
| `app/application/user/command/delete_user_test.go` | 5 | — | A |
| `app/application/user/command/superadmin_guard_test.go` | 2 | — | A |
| `app/application/user/command/update_user_test.go` | 8 | — | A |
| `app/application/user/command/zz_audit_test.go` | 7 | — | A |
| `app/application/user/query/list_users_test.go` | 3 | — | A |
| `app/application/userwrite/userwrite_test.go` | 5 | — | A |
| `app/infrastructure/sqlite/loadtest_test.go` | 0 | tag `loadtest`; `sqlite.NewManager`, vlastní DDL, `SQLITE_BUSY` | S |
| `app/infrastructure/sqlite/manager_test.go` | 0 | `sqlite.NewManager`, pool cap (WASM), `_txlock=immediate`, `"database is locked"`; test no-tx zóny je obecný → kontrakt | S |
| `app/infrastructure/sqlite/zz_audit_test.go` | 0 | `PRAGMA busy_timeout/foreign_keys/journal_mode`, `datetime('now','+1 hour')`; test kaskády tokenů → kontrakt | S |
| `app/infrastructure/sqlite/zz_gap_test.go` | 0 | `sqlite_master`, `PRAGMA index_list`, `goose.SetDialect("sqlite3")` + Down | S |
| `app/infrastructure/di/bus_integration_test.go` | 5 | import `sqlite/audit`, `provideCommandBus(…, fx.DB, …)`, `COUNT` audit/runs | A+ |
| `app/infrastructure/di/zz_audit_test.go` | 3 | import `sqlite/audit`, `provideCommandBus(…, fx.DB, …)` | A+ |
| `app/internal/repotest/audit/repository_test.go` | 1 | `SELECT … metadata … WHERE id=?`, porovnání bajtů → JSON | K |
| `app/internal/repotest/audit/zz_audit_test.go` | 0 | `fx.DB.BeginTx/Rollback`, raw `INSERT INTO audit_log … ?` (audit přežije rollback) | K |
| `app/internal/repotest/run/repository_cancel_test.go` | 9 | — (používá `forceExpire`) | K |
| `app/internal/repotest/run/repository_checkpoint_test.go` | 11 | 2 MiB blob, NUL bajty → `bytea` | K |
| `app/internal/repotest/run/repository_concurrency_test.go` | 12 | komentáře o busy_timeout; ms round-trip; `tnt-abc`, `T1/T2` | K |
| `app/internal/repotest/run/repository_fencing_test.go` | 15 | `tnt-123` | K |
| `app/internal/repotest/run/repository_tenant_test.go` | 2 | — | K |
| `app/internal/repotest/run/repository_test.go` | 21 | `forceExpire` (`strftime`), `julianday ± ms`, `fx.DB.BeginTx/Commit/Rollback`; precizní testy julianday → S část | K + S |
| `app/infrastructure/seeder/seeder_test.go` | 10 | import `sqlite/seeder`; `COUNT` tenants/audit | K |
| `app/infrastructure/seeder/zz_audit_test.go` | 2 | — | K |
| `app/internal/repotest/tenant/platform_test.go` | 1 | — | K |
| `app/internal/repotest/tenant/repository_test.go` | 3 | `does-not-exist` (N8) | K |
| `app/internal/repotest/token/repository_test.go` | 2 | — | K |
| `app/internal/repotest/token/zz_audit_test.go` | 2 | motivací je lexikální porovnání TEXT datetime; kontrakt je obecný | K |
| `app/internal/repotest/token/zz_gap_test.go` | 1 | raw `INSERT INTO refresh_tokens … ?`; assert textu `"NOT NULL"` (PG: „not-null") | K |
| `app/internal/repotest/user/platform_test.go` | 2 | — | K |
| `app/internal/repotest/user/read_one_test.go` | 4 | — | K |
| `app/internal/repotest/user/repository_test.go` | 5 | `UPDATE users SET locked_until = ? …` | K |
| `app/internal/repotest/user/save_tenant_test.go` | 1 | — | K |
| `app/internal/repotest/user/superadmin_guard_test.go` | 1 | — | K |
| `app/internal/repotest/user/tenant_isolation_test.go` | 4 | — | K |
| `app/internal/repotest/user/tenant_test.go` | 2 | `SELECT name FROM tenants WHERE id = ?` | K |
| `app/internal/repotest/user/zz_audit_test.go` | 2 | `fx.DB.BeginTx`, raw-pool zápis čeká na BEGIN IMMEDIATE write-lock | S |
| `app/internal/repotest/user/zz_gap_test.go` | 2 | `rawInsertUser` s `datetime('now')`, `active = 1` | K |
| `app/infrastructure/worker/run_timeout_withtx_test.go` | 5 | `COUNT(*) FROM runs … ?`; `fx.DB` jako Transactor | A+ |
| `app/infrastructure/worker/run_worker_audit_test.go` | 1 | `COUNT(*) FROM audit_log … ?` | A+ |
| `app/infrastructure/worker/run_worker_review_test.go` | 16 | `UPDATE runs SET run_at = strftime(…)` | A+ |
| `app/infrastructure/worker/run_worker_test.go` | 18 | `forceExpireW`/`stealLeaseW` (`strftime`), `UPDATE runs SET reclaims/parks`, `fx.DB.BeginTx`, `tenant-A` | A+ |
| `app/presentation/console/zz_audit_test.go` | 5 | — (komentář o globálním stavu goose) | A |
| `app/presentation/console/zz_gap_test.go` | 10 | `COUNT … audit_log … ?`; `--tenant-id no-such-id` | A+ |
| `app/presentation/http/handler/auth_test.go` | 1 | — | A |
| `app/presentation/http/handler/debug_run_test.go` | 2 | run id `does-not-exist` → 404 (N8) | A |
| `app/presentation/http/handler/profile_test.go` | 1 | — | A |
| `app/presentation/http/handler/zz_audit_test.go` | 2 | — | A |
| `app/presentation/http/server/zz_gap_test.go` | 1 | — | A |
| `app/zz_gap_test.go` | 0 | `sqlite.NewManager`, `sqlite_master`; životní cyklus `Application.Run` → přepsat přes `persistence` + `Migrator` | S → A+ |

**Mimo tabulku:**
- **Tři soubory importují `testfx` jen kvůli `NewJwt`** a DB nepoužívají: `app/presentation/http/middleware/auth_test.go`, `app/presentation/http/middleware/lang_test.go` a `app/presentation/http/server/zz_audit_test.go`. Přes `testfx` ale linkují SQLite driver, což řeší přesun `NewJwt` (7.2).
- **`app/internal/testfx/testfx.go`** sám obsahuje raw SQL `SELECT COUNT(*) FROM refresh_tokens` a `UPDATE tenants SET plan=? WHERE id=?` s `?`.

**Kontrolní součty:**
- 26 × A + 14 × A+ + 22 × K + 1 × K + S + 5 × S + 1 × S → A+ = **69 souborů**.
- Sloupec *fx* dává dohromady **336** volání `testfx.New*` v 63 souborech. K tomu 7 přímých volání `NewSqliteManager` v 5 souborech a 1 soubor (`sqlite/audit/zz_audit_test.go`), který DB dostává přes helper.
- Seznam vznikl dvěma nezávislými průchody (mechanický grep a ruční čtení každého souboru). Výsledky se shodly a tabulka byla proti nim zkontrolována strojově.

**Stav po fázi 2:**
- `app/internal/repotest/user/zz_audit_test.go` (kat. S) je kontrakt: raw-pool zápis čeká na zámek vnější tx a po jejím rollbacku přežije. Na Postgresu drží zámek řádek místo celé DB, test platí beze změny.
- `app/internal/repotest/run/repository_test.go` (kat. K + S) je celý kontrakt: posuny leasu o ±ms dělá `fx.SetLeaseFromNow` vůči hodinám databáze.
- Z SQLite adaptéru se do kontraktů přesunuly i dva obecné testy: no-tx zóna (`app/internal/repotest/tx/tx_test.go`) a kaskáda refresh tokenů při smazání uživatele (`app/internal/repotest/token/cascade_test.go`).
- `app/zz_gap_test.go` (kat. S → A+) nepotřebuje DB vůbec: pořadí „migrace před subpříkazem" ověřuje se zástupným `Migrator`em.
- V SQLite adaptéru zůstaly jen jeho vlastní testy: DSN pragmata, whitelist journal módů, pool cap, souběh `_txlock=immediate`, collation, `sqlite_master`/goose Down a gate testy nad jeho SQL.


## Příloha B — dokumentace k aktualizaci (fáze 7)

- **CLAUDE.md**: sekce Database, Environment, tabulka Infrastructure (`SqliteManager`, `sqlite/*`), vzor repozitáře a `wire.Bind`, checklist nové feature, invarianty (`*sqliteuser.Repository`, výjimky raw poolu, tenant scoping + RLS).
- **Skilly**: `/gk-repositories` (rozdělit na SQLite a Postgres část), `/gk-runs` (claim, split deploy, „roadmap krok 7"), `/gk-migrations` (dvojčata, Provider, `make migrate-*`), `/gk-testing` (`testfx.New(t)`, `make test-pg`, kategorie, pojistky), `/gk-multitenancy` (RLS, roviny, role), `/gk-config`, `/gk-deploy` (DSN proměnné, compose profil, bez volume `/data` na PG), `/gk-scheduler` (advisory lock), `/gk-rate-limiting`. Menší zmínky: `/gk-architecture`, `/gk-di`, `/gk-feature`, `/gk-bus`, `/gk-audit`, `/gk-auth`, `/gk-commands`, `/gk-entities`, `/gk-frontend-grid`, `/gk-init`, `/gk-queries`, `/gk`.
- **Framework docs**: `docs/framework/architecture.md` (startup sekvence), `docs/framework/configuration.md` (sekce Database / SQLite), `docs/framework/installation.md` (prerekvizity, migrace), `docs/framework/background/overview.md`, `docs/framework/background/durable-run.md`, `docs/framework/background/fire-and-forget.md`, `docs/framework/background/scheduler.md`, `docs/framework/gokick-roadmap.md` (Škálovatelnost: odkaz sem a rozhodnutí D3).
- **Ostatní**: `README.md`, `tests/e2e/README.md`, `.env.example`, `docker-compose.yml`, `docker/production/Dockerfile` (komentáře k `/data` a CGO).


## Související

- [Roadmap (GoKick)](/framework/gokick-roadmap) — sekce Škálovatelnost, ze které plán vychází.
- [Architektura](/framework/architecture) a [Konfigurace](/framework/configuration) — dnešní stav, který fáze 7 aktualizuje.
- Skilly: `/gk-repositories` (repozitáře, raw pool, tx), `/gk-runs` (durable fronta, lease, fencing), `/gk-multitenancy` (tenant scoping, conformance gate), `/gk-migrations`, `/gk-testing` (testfx, gate testy), `/gk-scheduler`, `/gk-di`.
