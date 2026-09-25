# Stav plánu — fáze 0–7

Zdroj pravdy: `docs/framework/postgres-adapter-plan.md` (sekce 9 = tabulka fází,
„Fáze 3/4 — co dopadlo jinak" = odchylky). Tady je souhrn + konkrétní návrhy pro
zbývající fáze.

## HOTOVO

### Fáze 0 — bugfix ✅ (v main)
- N1: pool cap se po startovních migracích ztrácel (SetMaxOpenConns(0) = neomezeno).
  Opraveno + regresní test. Drobné doc drifty.

### Fáze 1 + 1b — švy a české řazení (jen SQLite) ✅ (v main)
- Migrace `migrations/sqlite/` přes goose Provider API (bez globálního stavu).
- Driver-neutrální `app/infrastructure/database` (tx v ctx, port `Migrator`, `Driver`).
- `sqlite.Manager` / `sqlite.Migrator` v adaptéru; seeder v `infrastructure/seeder`.
- `persistence.Store` + `persistence.Open`; Wire bere porty přes `wire.FieldsOf`.
- SQLite per-connection `registerConnFuncs`: collation `app_sort` (cs-CZ), Unicode
  LIKE, SQL `uuidv7()`. UUIDv7 id všude. Zlatý korpus řazení
  `app/infrastructure/database/testdata/sort_cs/`. SQLite migrace squashnuté do 1 initu.

### Fáze 2 — testy nezávislé na DB ✅ (v main)
- `APP_DB_DRIVER` + `database.Driver`; SQLite adaptér celý za `//go:build !nosqlite`.
- Codemod `testfx.New(t)` (336 volání), `fx.Tx`, `fx.Audit`, pojmenované helpery
  (`app/internal/testfx/raw.go`), kontraktní testy portů v `app/internal/repotest/<ctx>`.
- Gate `app/zz_nosqlite_test.go`, `make nosqlite-check` (součást `make lint`).

### Fáze 3 — Postgres základ ✅ (PR #66, mergnuto)
- `shared.Plane` (tenant = nulová hodnota, platform, system), `PlaneMiddleware`,
  `SystemPlaneMiddleware`, `ReadTxMiddleware` + `Transactor.BeginReadTx`.
- `migrations/postgres/20260327000001_init_schema.sql`: nativní typy, FK (i
  `runs.tenant_id … ON DELETE CASCADE`), ICU collation `app_sort`, RLS (`ENABLE`, ne
  `FORCE`), granty pro `gokick_app` / `gokick_system`; audit append-only.
- `postgres.Manager`: dva pgx pooly (app = RLS, system = BYPASSRLS), tx-lokální
  `set_config('app.tenant_id', …, true)`, `BeginReadTx` READ ONLY a join jen při stejném
  scope; session timeouty; `VerifyRoles` (odmítne superusera/BYPASSRLS/vlastníka i
  členství v nich). `postgres.Migrator` jako `gokick_owner` pod advisory lockem.
- Docker: `docker/postgres/Dockerfile`, `initdb/01-roles.sh`, compose služby `db`
  (persistentní) a `db-test` (tmpfs, fsync off), bez portů; `make db-up` v build/serve,
  `db-down/db-reset/db-psql`, `make test-pg`, CI job `postgres tests`.
- Harness `app/internal/testfx/pgfx` (šablona `gokick_tpl_<hash>` + klon per test).
- RLS sada `app/infrastructure/postgres/rls_test.go`.
- Po review opravy (join scope, členství v elevated roli, sub-ms timeout, cascade,
  index s `app_sort`, RLS na všech tabulkách) + compose hesla + `make test` pinuje sqlite.

### Fáze 4 — Postgres repozitáře ✅ (PR #67, otevřený)
Commity (každý ověřen samostatně v čistém worktree):
1. `109e2cb refactor(db): share the portable SQL helpers in the database package`
   — `database/sql.go`: `CollateSort`, `LikeEscape`, `LikeContains`, `MsPrecisionUTC`,
   `NotTerminalClause`, `RowsAffectedBool`.
2. `739abaa feat(bus): run login and refresh on the system plane` — `shared.PreTenant`
   marker; `PlaneMiddleware` dá systémovou rovinu commandu s `PreTenant`+`SkipPermission`;
   allow-list gate `app/application/zz_pretenant_test.go`; test přes bus
   `app/application/auth/command/pretenant_test.go`.
3. `1db4328 test: call handlers and repositories with the plane the bus would give them`
   — `testfx.TenantCtx/PlatformCtx` (+ existující `SystemCtx`); config testy mažou `APP_*`.
4. `063c1ed feat(db): report a taken nickname or tenant name as its field error`
   — 23505 / SQLite CONSTRAINT_UNIQUE → `ValidationError` (nickname / name) na obou
   adaptérech; kontraktní testy `repotest/{user,tenant}/constraint_test.go` + malformed id.
5. `ef349a6 feat(postgres): add the Postgres repositories and make the adapter selectable`
   — `infrastructure/postgres/{user,tenant,token,run,audit}`, `BaseRepository`
   (`conn.go`), `sql.go` (`Args`, `NowExpr`, `NowPlus`, `ILikeContains`,
   `NullsSmallest`), UTC timestamptz codec, `persistence.PostgresStore`, testfx PG
   backend, arch-lint `postgres_repos`, `make test-pg` = celá suite.
6. `5db256e test: gate both adapters' SQL for tenant scope, the clock and the system role`
   — `app/zz_tenant_test.go` (oba adaptéry), `app/zz_sqltime_test.go` (přesun),
   `app/zz_pgtime_test.go`, `app/zz_pgsystem_test.go`.
7. `773eb14 docs: mark phase 4 of the Postgres adapter plan done`.

Ověřeno: celá suite zelená na SQLite i PG16 lokálně; `-race` čistý; mutační kontroly;
smoke test binárky `-tags nosqlite` nad PG (multitenant seed, create-user, serve, login,
admin grid jen svůj tenant, platform grid napříč tenanty). Neověřeno lokálně: PG 18
(ověří CI job), frontend lint, documan-lint (potřebuje Docker).

---

## ZBÝVÁ

### Fáze 5 — zámky a souběh (další na řadě)
Plán: sekce 5 + tabulka fází. „Hotovo, když": testy souběhu zelené na PG a opakovaně
(`-count=20`).

1. **Advisory `Locker` pro scheduler** (N14): dnes každá replika `serve` spustí každý
   job. Port `shared.Locker` (např. `TryWithLock(ctx, key string, fn func(ctx) error)
   (ran bool, err error)`), na Postgresu `pg_try_advisory_lock(hash(key))` na
   **vyhrazeném spojení** (session lock → potřebuje pinned `*sql.Conn`, pak
   `pg_advisory_unlock`), nevlastník tick přeskočí; na SQLite in-process no-op (vždy
   `ran=true`). Přidat na `persistence.Store` (`Locker`), do Wire `FieldsOf`, do
   `provideScheduler` / `scheduler.NewScheduler`. Klíč `job:<name>`.
   Upravit `TestScheduler_TwoInstancesTickIndependently`
   (`app/infrastructure/scheduler/scheduler_test.go:229`) — dnes výslovně dokumentuje,
   že dvě instance tikají nezávisle. Scheduler job běží bez roviny → sweep tokenů už
   jde přes `SystemConn`.
2. **Retry při 40001 / 40P01 / 55P03**: dnes z toho je 500. Klasifikace chyb
   (např. `database.ErrRetryable` / `postgres.IsRetryable`), `TransactionMiddleware`
   (`app/application/bus/middleware/transaction.go`) zopakuje celý handler (max 3×,
   jitter) — bezpečné, protože eventy se dispatchují až po commitu a audit je mimo tx.
   Volitelně marker `shared.RequiresSerializable` → tx `SERIALIZABLE` na PG (SQLite
   no-op). Plán doporučuje začít bez něj.
3. **Zámek na „kotvě"** pro invarianty přes víc řádků (guard superadmina v
   `app/application/userwrite/userwrite.go`, budoucí „poslední admin"):
   `SELECT … FROM tenants WHERE id=$1 FOR UPDATE` nebo `pg_advisory_xact_lock`.
4. **Deterministické pořadí u bulk operací**: `… WHERE id IN (SELECT id … ORDER BY id
   FOR UPDATE)`; deadlock 40P01 je retryovatelný. Známý případ: bulk delete tenantů
   (`BulkDeleteEmptyAcrossTenants`) při souběžném insertu uživatele skončí 23503 → 500
   (single delete už 23503 mapuje na „není prázdný").
5. **PG testy souběhu**: N workerů × M runů exactly-once (SKIP LOCKED, reálně paralelně),
   dvě instance scheduleru (job běží jednou), souběžné migrace (už existuje
   `TestMigrator_ConcurrentRunUp`), souběžné commandy bez ztráty; spouštět s `-count=20`.
   PG dvojče SQLite testu `TestManager_ConcurrentTxWritesDoNotReturnBusy`.
6. Docs k fázi 5 (plán, `/gk-scheduler`, `/gk-runs`, CLAUDE.md).

### Fáze 6 — CI a pojistky
Plán: sekce 7.1, 7.7. „Hotovo, když": oba joby povinné v branch rulesetu; `make test`
pouští obě DB.

1. **Povinné checky**: `scripts/setup-github.sh` (ruleset, ř. ~176) má required
   contexts `lint + test + build`, `durable-run E2E`, `Lint docs` → přidat
   `postgres tests`. Pozor na komentář ve skriptu: required context musí být
   reportovatelný i na release-please PR (joby mají job-level `if` pro
   `release-please--` větve — ověřit, jak to skript řeší pro ostatní joby a zopakovat).
   Ruleset aplikuje uživatel (`make setup-github`, potřebuje `gh`) — v cloud session
   `gh` není; změnu skriptu udělat, aplikaci nechat na uživateli.
2. **`make test` = obě DB paralelně**: `docker compose up -d --wait db-test`, dva
   `go test` procesy (`APP_DB_DRIVER=sqlite` a `APP_DB_DRIVER=postgres -tags nosqlite`),
   výstupy prefixované `[sqlite]`/`[postgres]`, selhání kterékoli = selhání. Bez Dockeru
   srozumitelně selhat a nabídnout `make test-sqlite` (nový target). `make test-pg`
   zůstává. Dnes `make test` pinuje `APP_DB_DRIVER=sqlite`.
3. **Kontrola artefaktů**: PG běh s `TMPDIR` v čerstvém adresáři, po běhu žádný
   `*.db`, `*-wal`, `*-journal`.
4. **Lint pro obě sady tagů** — už je (`nosqlite-check` v `make lint`).
5. **E2E na Postgresu**: `tests/e2e/lib.sh` PG varianta; `at_least_once.sh` volá
   `sqlite3` (ř. 20–21) → `psql` nebo počet přes `/debug/runs`. Odložený e2e
   **multi-process fencing** (dva procesy workeru nad jednou DB). Job `durable-run E2E`
   ve `.github/workflows/validate.yml`.

### Fáze 7 — dokumentace
Plán: Příloha B. Část už průběžně hotová (CLAUDE.md, gk-repositories, gk-testing,
gk-multitenancy, gk-config, gk-frontend-grid, configuration/installation/architecture,
roadmap řádek). Zbývá hlavně:
- Skilly: `/gk-runs` (claim SKIP LOCKED, split deploy), `/gk-migrations` (dvojčata,
  Provider, `make migrate-*`), `/gk-deploy` (DSN proměnné, compose profil, bez volume
  `/data` na PG), `/gk-scheduler` (advisory lock), `/gk-rate-limiting`, drobné zmínky
  v `/gk-architecture`, `/gk-di`, `/gk-feature`, `/gk-bus`, `/gk-audit`, `/gk-auth`,
  `/gk-commands`, `/gk-entities`, `/gk-init`, `/gk-queries`, `/gk`.
- Framework docs: `docs/framework/background/{overview,durable-run,fire-and-forget,
  scheduler}.md`, architecture (startup sekvence), installation (prerekvizity).
- Ostatní: `README.md`, `tests/e2e/README.md`, `docker/production/Dockerfile`
  (komentáře k `/data` a CGO).
- Zapsat rozhodnutí D1–D10 do roadmapy / dokumentace.
- Kontrola: `make docpaths-check` + `documan-lint` zelené.

### Mimo fáze / volitelné
- `make migrate-up/down/status` obsluhují jen SQLite (na PG migruje aplikace při
  startu). Plán 3.3: `goose -env` nebo `gk` subcommand.
- `LISTEN/NOTIFY` probuzení workeru; produkční build `-tags nosqlite`; sdílený stav
  rate-limiteru (dnes per replika v paměti); `t.Parallel()` v DB testech;
  self-hosted runner pro privátní projekty ze šablony.
- N10: zdokumentovat požadavek NTP (run_at/expires_at píše Go, lease počítá DB).
- Kontraktní test SQL seedů na obou DB (4.3) — zatím žádné SQL seedy neexistují.
