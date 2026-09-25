# Technické poznámky — architektura adaptéru, pasti, prostředí, konvence

## 1. Jak adaptér funguje (stav po fázi 4)

- **Volba adaptéru**: `persistence.Open(cfg)` větví podle `APP_DB_DRIVER`
  (`persistence/sqlite.go` je `//go:build !nosqlite`, `persistence/postgres.go` vždy).
  `testfx.New(t)` dělá totéž (`testfx/sqlite.go` / `testfx/postgres.go`) —
  `persistence.SQLiteStore` / `PostgresStore` = stejné zapojení jako produkce.
- **Roviny** (`shared.Plane` v ctx): `PlaneMiddleware` z permission (`platform:*` →
  platform, jinak tenant); `SystemCommandBus` → system; `shared.PreTenant` (jen
  `LoginCommand`, `RefreshTokenCommand`, allow-list gate) → system. Worker a scheduler
  rovinu nenastavují; run handler běží v tenantové rovině svého runu (tenant z řádku).
- **Postgres `BaseRepository`** (`app/infrastructure/postgres/conn.go`):
  - `Conn(ctx)`: tx z ctx → jinak systémový pool na platform/system rovině → jinak
    `scopedConn` = transakce na jeden příkaz na app poolu, scopnutá na tenanta
    (`BEGIN; set_config; stmt; COMMIT`). Bez tenanta v multitenant módu `errConn`.
  - `SystemConn(ctx)`: systémová role vždy; připojí se k tx z ctx jen když je
    cross-tenant (txScope v ctx). Pro `*AcrossTenants`, worker bookkeeping
    (`ClaimDue`, `RenewLease`, `fenced`), `DeleteExpired`, `FindByNickname`.
  - `r.DB.System()`: raw systémový pool = dvojče SQLite raw poolu (login čítače, audit).
  - Allow-list metod se systémovou rolí: `app/zz_pgsystem_test.go`.
- **SQL konvence PG repozitářů**: `$n`; dynamické filtry přes `postgres.Args`
  (fixní argumenty předem: `a := postgres.Args{tenantID}`, hlava statementu jako jeden
  literál s `$1`); hodiny jen `postgres.NowExpr` / `NowPlus("$n")`
  (`statement_timestamp()`); textové filtry `postgres.ILikeContains`; nullable sort
  sloupec + `postgres.NullsSmallest(dir)`; id z venku `postgres.ParseID`/`ParseIDs`;
  `id = ANY($n)` pro seznamy id.
- **Chyby**: 23505 na `users_nickname_key` / `idx_tenants_name` → `ValidationError`
  (i SQLite podle `CONSTRAINT_UNIQUE`); 23503 v `DeleteIfEmptyAcrossTenants` →
  `(false, nil)`; 42501 = bug (interní chyba); 40001/40P01/55P03 zatím neřešeno (fáze 5).
- **RLS**: `gokick_current_tenant()` s `NULLIF`; politiky na users/runs/tenants(SELECT
  vlastní)/refresh_tokens(přes viditelnost uživatele); audit_log bez politiky a bez
  grantu pro app roli.

## 2. Poučení / pasti (ušetří hodiny)

- **Holý `context.Background()` v testu = tenantová rovina výchozího tenanta.** Na
  SQLite projde cokoli, na PG RLS ukáže jen výchozí tenant. Test, který volá handler
  nebo repozitář bez busu, musí dát ctx jako bus: `testfx.TenantCtx(id)`,
  `testfx.PlatformCtx()`, `testfx.SystemCtx()` (seed a inspekce cizího tenanta).
- **Tenant gate čte SQL po jednotlivých string literálech** — statement složený
  z více literálů se kontroluje po kouscích. Hlava (`UPDATE users SET … WHERE
  tenant_id = $1`) musí být v jednom literálu, marker `/* tenant-scope-exempt: … */`
  ve stejném literálu jako `FROM users` / `UPDATE users` (dej ho na začátek statementu).
- **sqlx named queries** (`NamedExecContext`): `:` v komentáři se bere jako bind →
  marker psát `/* tenant-write-exempt - důvod */` (pomlčka); `::` cast v named query
  sqlx přepíše — v named queries casty nepoužívat.
- **`const q = …` s voláním funkce** (`postgres.NowPlus`) neprojde → `q := …`.
- **`LIKE 'gokick_t_%'` v psql**: `_` je wildcard, matchne i `gokick_tpl…` (šablonu).
- **Config testy** čtou prostředí procesu — `TestMain` v `config_test.go` maže
  zděděné `APP_*`, jinak do nich pronikne `APP_DB_DRIVER=postgres` z PG běhu.
- **pgx timestamptz** se vrací v lokální zóně → registrovaný codec
  `TimestamptzCodec{ScanLocation: time.UTC}` v `openPool` (`OptionAfterConnect`).
- **`go vet`/lint v nosqlite**: soubory SQLite adaptéru jsou tagované; gaty, které
  používají helpery z jiných test souborů, se musí kompilovat v obou tag sadách
  (proto `app/zz_sqltime_test.go` je `!nosqlite` a helpery bere ze `zz_tenant_test.go`).
- **zz_nosqlite gate** zakazuje SQLite dialekt (`julianday(`, `strftime(`,
  `datetime(`, `PRAGMA`, `sqlite_master`, `INSERT OR`, `*.db`) v netagovaných
  testech a fixtures — nepiš ho do sdílených testů.
- **Race běh** `auth/command` trvá ~6,5 min (bcrypt) — dej timeout.
- Commit na pozadí: nesahej na soubory, dokud běží `go test` na pozadí (kompiluje
  balíčky postupně).

## 3. Lokální prostředí (cloud container)

- Docker v containeru není; lokálně je **PostgreSQL 16 cluster** (image 18 se stáhnout
  nedal). Start: `pg_ctlcluster 16 main start` (mezi turny občas spadne).
  Role `gokick_owner/app/system` s hesly = jméno role už v clusteru jsou
  (init skript `docker/postgres/initdb/01-roles.sh` je idempotentní).
- Superuser DSN pro testy:
  `APP_TEST_DB_URL="postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"`
- Nástroje: `$(go env GOPATH)/bin` = golangci-lint (v PATH je jiná, stará binárka
  pro go1.25 — vždy volej `$(go env GOPATH)/bin/golangci-lint`), golines, goimports,
  go-arch-lint, wire, goose, lefthook. `goimports -local gokick -w <file>`.
- `make lint` celé lokálně neprojde (yarn/corepack proxy 403) → pouštěj Go části
  jednotlivě (viz níže). `documan-lint` potřebuje Docker → ověří CI.
- `gh` CLI není; GitHub přes MCP nástroje `mcp__github__*` (ToolSearch je načte).

## 4. Ověřovací příkazy

```bash
# SQLite suite
APP_DB_DRIVER=sqlite go test ./app/... ./cmd/...
(cd tools/gk && go test ./...)
# Postgres suite (celá, bez SQLite v buildu)
APP_TEST_DB_URL=… APP_DB_DRIVER=postgres go test -tags nosqlite -timeout 20m ./app/... ./cmd/...
# race na PG částech
APP_TEST_DB_URL=… APP_DB_DRIVER=postgres go test -race -tags nosqlite ./app/infrastructure/postgres/... ./app/internal/... ./app/infrastructure/worker/...
# lint (Go části)
$(go env GOPATH)/bin/golangci-lint run ./app/... ./cmd/...
make nosqlite-check arch-check format-check boundary-check errfields-check docpaths-check
# žádné zbytky DB po testech (0 = ok; šablona gokick_tpl_* smí zůstat)
psql "$APP_TEST_DB_URL" -Atc "select count(*) from pg_database where datname ~ '^gokick_t_[0-9a-f]'"
```

- Každý commit ověřit samostatně v čistém worktree (`git worktree add --detach …`):
  build + vet pro oba tag sety, SQLite suite, PG suite. Skript z minulé session to
  dělal smyčkou přes `git rev-list --reverse origin/main..HEAD`.
- Mutační kontrola nových testů: dočasně vrátit opravu → test musí spadnout → vrátit.
- Smoke test binárky: `go build -tags nosqlite -o app ./cmd/`, prázdná DB
  `CREATE DATABASE x OWNER gokick_owner`, env `APP_DB_DRIVER=postgres APP_DB_URL=…
  APP_DB_SYSTEM_URL=… APP_DB_MIGRATE_URL=… APP_JWT_SECRET=<32+> APP_MULTITENANCY=true …`,
  `./app seed`, `./app serve`, `curl` login (`POST /api/v1/auth/login`, odpověď má
  `access_token`), `GET /api/v1/admin/users`, `GET /api/v1/platform/users`.

## 5. Konvence commitů a PR

- Conventional Commits (`feat`/`fix`/`refactor`/`test`/`docs`/`build`/`ci`/`chore`,
  scope např. `db`, `postgres`, `bus`); PR se **rebase-mergují** → každý commit čistý,
  samostatně zelený. Commit body anglicky, věcně (co a proč).
- Patička každého commitu (přesně):
  ```
  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: <URL aktuální session>
  ```
  (Předchozí session: `https://claude.ai/code/session_012Jn1C5X6byzXkCLQw7dHEw` —
  nová session použije svou URL, pokud ji systém dodá.)
- PR popis anglicky, struktura jako #66/#67: úvod, „Commits" (číslovaně s názvy
  commitů a odrážkami), „Verification" (co bylo spuštěno a co ne). Konec:
  `🤖 Generated with [Claude Code](https://claude.com/claude-code)` + URL session.
  Nikdy neuvádět identifikátor modelu v commitech/PR.
- Push: `git push -u origin feature/kind-planck-fk3df4`. Po merge PR se větev na
  GitHubu smaže (auto-delete) → nový push bez force.
- Po založení PR: přihlásit se k událostem PR a hlídat CI do zelena (oprava =
  root cause, lokálně ověřit, pak push).

## 6. Mapa klíčových souborů

- Plán: `docs/framework/postgres-adapter-plan.md`
- Postgres adaptér: `app/infrastructure/postgres/{manager,conn,sql,migrator,roles}.go`,
  repozitáře `app/infrastructure/postgres/{user,tenant,token,run,audit}/`
- SQLite adaptér: `app/infrastructure/sqlite/…` (vše `!nosqlite`)
- Neutrální: `app/infrastructure/database/{tx,sql,sort,driver,migrator}.go`,
  `app/infrastructure/persistence/`
- Roviny: `app/domain/shared/plane.go`, `app/application/bus/middleware/{plane,read_tx,transaction,tenant}.go`
- Test harness: `app/internal/testfx/{testfx,raw,sqlite,postgres,driver}.go`,
  `app/internal/testfx/pgfx/pgfx.go`, kontraktní testy `app/internal/repotest/<ctx>/`
- Gaty: `app/zz_tenant_test.go`, `app/zz_sqltime_test.go`, `app/zz_pgtime_test.go`,
  `app/zz_pgsystem_test.go`, `app/zz_nosqlite_test.go`, `app/zz_migrations_test.go`,
  `app/application/zz_pretenant_test.go`, `app/application/zz_platform_isolation_test.go`
- Migrace: `migrations/{sqlite,postgres}/20260327000001_init_schema.sql`
- Docker/CI: `docker/postgres/`, `docker-compose.yml`, `Makefile` (`db-*`, `test-pg`),
  `.github/workflows/validate.yml`, `scripts/setup-github.sh` (ruleset)
- Scheduler (fáze 5): `app/infrastructure/scheduler/`, joby v
  `app/infrastructure/di/container_provider.go` (`provideSchedulerJobs`)
