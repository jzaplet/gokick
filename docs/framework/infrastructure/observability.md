---
layout: 'page'
uri: '/framework/infrastructure/observability'
position: 7
slug: 'framework-infrastructure-observability'
parent: 'framework-infrastructure'
navTitle: 'Observability'
title: 'Observability'
description: 'Strukturované logování, jednotná slovní zásoba atributů, korelace přes trace_id/user_id a připravený šev pro OpenTelemetry.'
---

# Observability

Aplikace loguje strukturovaně přes Go `log/slog` do stderr. Tato stránka popisuje konvenci atributů, jak vzniká korelace mezi logy, a kde do systému zapadne OpenTelemetry (traces/metrics), až bude potřeba — bez přepisování call sites.


## Strukturované logy

- **Formát a level** řídí `APP_LOG_FORMAT` (`json` — default, pro agregátory jako Loki; `text` — čitelné pro lokální `make serve`) a `APP_LOG_LEVEL` (`debug` / `info` — default / `warn` / `error`). Neznámé hodnoty degradují na `json` / `info`.
- **Logger se staví na jediném místě** — `newLogger` v `cmd/logger.go` (přes testovatelný `newLogHandler`). Nikde jinde se `*slog.Logger` nevytváří; všude se injektuje přes Wire DI z `main.go`. To je záměrně jediný šev (viz níže).
- `.env` se načte v `main.go` ještě před stavbou loggeru, aby `APP_LOG_*` platily i lokálně.


## Jednotná slovní zásoba atributů

`app/domain/shared/log.go` definuje konstanty klíčů, aby napříč vrstvami nevznikaly varianty téhož pole:

| Klíč | Význam |
|---|---|
| `trace_id` | korelační ID requestu (z `TraceMiddleware`) |
| `user_id` | ID autentizovaného uživatele (z `AuthClaims`) |
| `command` | jméno command/query na busu |
| `duration_ms` | doba trvání ve zlomku ms (µs přesnost, číselné) |
| `retry_in_ms` | odklad dalšího pokusu jobu |
| `error` / `event` / `job_kind` | chyba / jméno domain eventu / druh jobu |
| `method` / `path` / `url` / `user_agent` | HTTP request — putují z presentation vrstvy i do error reporteru (Sentry `event.Request`) |

Pravidlo: **každý klíč je Go konstanta** (vynuceno staticky, viz níže) — nikdy holý string literál. Cross-cutting klíče jsou v `shared.LogKey*` (`trace_id`, `user_id`, `command`, `duration_ms`, `error`, `event`, `job_kind`, `retry_in_ms`, `method`, `path`, `url`, `user_agent`); komponentně-specifické klíče jako package-local `logKey*` konstanty v dané komponentě (`addr`/`timeout` v serveru, `ip`/`status`/`bytes` v access logu, `slot`/`job_id`/`attempts` ve workeru, `from`/`to`/`version` v migracích…). `domain/shared` tak nezná infra klíče. Korelaci produkuje `shared.LogAttrs`, dobu `shared.DurationMsAttr`.

`method`/`path`/`url`/`user_agent` jsou cross-cutting schválně: `RecoveryMiddleware` je předá do `ErrorReporter.Capture` a Sentry adaptér z nich (a jen z nich — pevný whitelist) složí `event.Request`. Producent (HTTP middleware) i konzument (`cmd/sentry.go`) tak sdílí slovník, aniž by jeden importoval druhého.


## Statické vynucení

Logování má **jedinou cestu** a žádný vývojář ani AI ji nemůže nepozorovaně obejít — `.golangci.yml` to hlídá při lintu (a tím v CI). Konkrétně:

- **`depguard` — import allow-list** (ne deny-list): povolen jen stdlib (`$gostd`), `gokick` a explicitně vyjmenované přímé závislosti. Tím padá **celá třída** cizích loggerů jedním tahem — `charmbracelet/log`, `glog`, `hclog`, `apex/log`, `go-kit/log`, `log15`, OTel logs SDK i vendored fork — neprojdou už importem. `log/syslog` je navíc explicitně deny. *Výjimka:* `getsentry/sentry-go` je v allow-listu **vědomě** — je to jediný sankcionovaný non-slog sink (error tracking, viz níže), ne logger. *Daň:* nová závislost = nový řádek v allow (stejná disciplína jako `.go-arch-lint.yml`).
- **`forbidigo`** — zakázaná volání: `fmt.Print*` + `print`/`println` (stdout), stdlib `log.*` (vč. `log.New`/`log.Default`), `slog.New*` (konstrukce loggeru/handleru) mimo `cmd/`, `slog.Default()` (chain bypass `no-global`), `os.Stdout`/`os.Stderr`, **`os.Create`/`os.OpenFile`/`os.WriteFile`/`os.NewFile`** (otevření souboru/fd — přesně „logování do souboru") a `syscall.Write`.
- **`sloglint`** — `no-global` (žádný globální default logger — jen injektovaný), `static-msg` (zprávy konstantní), `no-raw-keys` (každý klíč konstanta), `key-naming-case: snake`, `no-mixed-args` (nemíchat kv páry a `slog.Attr`).

Výjimky (úzké, přes `linters.exclusions`): `presentation/console/` smí `fmt.Print` (CLI výstup pro uživatele), `cmd/` smí `slog.New` + `os.Stderr` (konstruktor loggeru), `internal/testfx/` a `*_test.go` jsou z forbidigo/sloglint vyňaté, `domain/shared/log.go` definuje klíče a key-parametrizované helpery (tedy mimo `no-raw-keys`).

Rozsah byl ověřen adverzariálně (red-team probe): zavřené jsou všechny *náhodné* vektory — logování do souboru, `fmt.Fprintf` na soubor (zdroj souboru je zakázán), cizí logger, `slog.Default().Info()`, druhý slog logger. **Reziduum (vědomě, mimo dosah name-based lintu):** odhodlaný bypass přes `net.Dial` socket sink, `go:linkname`/raw runtime, nebo zápis na fd získaný cestou, kterou linter nepojmenuje.

> Statická analýza zastaví *náhodný* drift (o ten tu jde), ne odhodlaný bypass. A vynucuje *call-site* disciplínu, ne runtime doručení (ztráta při pádu / zachytávání stderr je ops). Pozn.: CI instaluje `golangci-lint@latest` (build s Go z `go.mod`), takže nehrozí version skew, kdy by se lint tiše neprovedl.


## Korelace: `LogAttrs(ctx)`

`shared.LogAttrs(ctx) []slog.Attr` je **jediný** zdroj korelačních atributů — vrátí `trace_id` (když je) a `user_id` (u autentizovaných requestů). Skládá se přímo s metodou `logger.LogAttrs`, takže není potřeba žádná `[]any` konverze:

```go
attrs := append(shared.LogAttrs(ctx), slog.String(shared.LogKeyCommand, name))
logger.LogAttrs(ctx, slog.LevelInfo, "bus: completed",
    append(attrs, shared.DurationMsAttr(d))...)
```

- `user_id` je dostupné na **bus vrstvě** — claims injektuje HTTP `AuthMiddleware` ještě před voláním busu. Pro login/refresh (neautentizované, `SkipPermission`) se `user_id` vynechá.
- Globální HTTP `LoggingMiddleware` běží **před** auth → nese `trace_id`, ne `user_id`. To je v pořádku — spolehlivá vrstva pro `user_id` je bus. Access log line (`http: request`) navíc nese `method`, `path`, `ip` (rozlišená klientská IP, viz [Config](/framework/infrastructure/config#app_trust_proxy_headers--cloudflare-origin-lock)), `status` a `bytes` (z `statusRecorder`, který obalí `ResponseWriter`) a `duration_ms` — request origin a výsledek jsou tedy v logu, ne jen v audit trailu.
- Dobu vždy loguj přes `shared.DurationMsAttr(d)` — číselné `duration_ms`, ne `time.Duration` (které se v JSON serializuje jako nanosekundy).


## Sentry — chyby & paniky

Tahle sekce popisuje, jak error reporting funguje **uvnitř**. Provozní nastavení (založení projektů, DSN, dev vs prod delivery, deploy za Cloudflare, ověření) je v [Sentry guide](/guides/sentry).

Neočekávaná selhání se hlásí do Sentry. **Není to logovací cesta** — běžné návratové chyby (validace, auth, 4xx) se sem nehlásí, jen recovery/terminal cesty, jinak tracker utone v šumu.

- **Port `shared.ErrorReporter`** (`Capture(ctx, err, attrs...)`, `Flush`). Bez DSN → `NopReporter` (no-op), takže appka běží beze změny i bez Sentry účtu. Staven v `cmd/sentry.go` (jako logger) a injektovaný, takže sentry-go import zůstává mimo vrstvený `app/` strom.
- **BE hooky:** bus `RecoveryMiddleware`, **HTTP `RecoveryMiddleware`** (panika → log + report + 500), worker (exhausted retries). Reporter přidá `trace_id`/`user_id` z ctx + předané tagy (`command` / `job_kind` / `method` / `url` / `user_agent`).
- **Obohacení eventu** (`cmd/sentry.go`):
  - **User** — `id` / `nickname` (username) / `role` z `AuthClaims` v ctx + rozlišená klientská IP (`user.ip_address`). Sentry tak chyby atribuuje a grupuje podle toho, kdo na ně narazil; i pre-auth paniky nesou aspoň IP. Vyžaduje `SendDefaultPII: true` v `Init` — bez něj sentry-go ručně nastavenou IP zahodí. Nic se tím neauto-sbírá: žádná HTTP integrace není nainstalovaná a `event.Request` se skládá z whitelistu níže.
  - **Request** — `method` / `url` / `User-Agent` z předaných attrs, **nikdy** ze syrové hlavičkové sady (ta nese `Authorization`/`Cookie`). Skládá se jen z tohoto pevného whitelistu; bez `method` (non-HTTP caller, např. worker) se `event.Request` nenastaví.
  - **Paniky** — bus i HTTP recovery balí recovered hodnotu do `shared.PanicError` (doménový typ, bez sentry importu). Adaptér z ní udělá exception type `panic` (místo generického `*errors.errorString`) + tag `panic.type` (konkrétní Go typ — `string` vs `runtime.Error`). Zároveň **degraduje naše reporting framy** (`sentryReporter.Capture`, `*RecoveryMiddleware`) na `in_app:false`, takže culprit/titulek míří na reálné místo paniky, ne na náš reporter. (Negroupuje to jinak — grouping jede přes všechny framy; mění to jen „kde to je".)
  - **Breadcrumbs** — stopa `INFO+` log řádků vedoucích k chybě (jako Symfony Monolog/Doctrine trail). `WithRequestScope(ctx)` (port `ErrorReporter`) naváže per-request sentry hub na ctx; `breadcrumbHandler` v `cmd/logger.go` (obal slog handleru) přidá každý `INFO+` záznam na ten hub a `Capture` použije hub z ctx (s nasbíranou stopou). Scope zakládá `ReportScopeMiddleware` (HTTP, před Recovery) a worker per-job. **Jen ctx-form logy** (`LogAttrs(ctx, …)`) breadcrumb nesou — bus i HTTP hot path je používají; `NopReporter` scope neřeší, takže bez Sentry je to pass-through.
- **Lifecycle:** `Init` při startu, `defer Flush` v `main` (a explicitně před `os.Exit`) — `CaptureException` je async, panika / `os.Exit` by jinak event ztratily.
- **FE:** `@sentry/vue` (`assets/app-ui/Sentry/`), init v `app.ts` před prvním `await`. Zachytává Vue chyby + unhandled promise rejections; handled API 4xx z `authFetch`/`apiFetch` se nehlásí. Browser SDK navíc sbírá breadcrumbs automaticky (kliky, navigace, konzole). `syncSentryUser` drží `Sentry.setUser` v zámku se session — `watch` nad jediným `user` ref, takže login / refresh / logout / `clearAuth` ho aktualizují a žádná auth cesta nemůže zapomenout.
- **Config:** `APP_SENTRY_DSN` + `APP_SENTRY_ENVIRONMENT` + `APP_SENTRY_RELEASE` (BE). FE a BE jsou dva Sentry projekty → dva DSN; FE DSN se předává jako `APP_SENTRY_DSN_FRONTEND`. Aby jediný build sloužil všem prostředím, Go server injektuje FE hodnoty (DSN, environment, debug flag) do `index.html` jako `<meta name="gokick:…">` tagy a SPA je čte za běhu (`runtimeConfig.ts`); `VITE_SENTRY_*` jsou jen build-time fallback pod Vite dev serverem. CSP `connect-src` se otevře na ingest origin DSN, jinak prohlížeč FE eventy zablokuje. `APP_SENTRY_DEBUG=true` zapne záměrné triggery chyb (BE `GET /debug/sentry` panika + FE tlačítko) pro smoke-test Sentry — **nikdy v produkci** (appka při startu varuje).
- **Release verze (z git tagu):** stampuje se při buildu — do binárky přes `-ldflags "-X main.release=<tag>"` (`cmd/version.go`, fallback `APP_SENTRY_RELEASE`) a do SPA bundlu přes `VITE_SENTRY_RELEASE`. Lokálně `make build` bere `git describe --tags`; release workflow tag. Tím se Sentry issues grupují podle nasazené verze. Verze se loguje i na startu (`starting gokick version=…`).
- **Release workflow** (`.github/workflows/release.yml`, na `v*` tag): postaví produkční image s verzí z tagu. **Push do GHCR je default vypnutý** (gokick je šablona) — povolíš repo proměnnou `RELEASE_PUSH=true` (Settings → Actions → Variables), žádný secret (GHCR jede přes `GITHUB_TOKEN`). Bez ní se image jen postaví (ověří release build), nepushne.

- **Source maps (FE):** `@sentry/vite-plugin` nahraje source mapy při buildu a hned je **smaže z dist** (`filesToDeleteAfterUpload`), takže se neembednou do binárky (která `public/` servíruje); Dockerfile to navíc jistí guardem. Symbolizace jede na debug-ID. Opt-in přes build-time `SENTRY_AUTH_TOKEN` (secret) + `SENTRY_ORG`/`SENTRY_PROJECT` (vars) — bez nich build mapy nevytvoří. Operátorský postup: [Sentry guide → Source maps](/guides/sentry#source-maps).


## Šev pro OpenTelemetry

Systém je připravený tak, aby OTel šel doplnit **lokalizovaně**, bez zásahu do jednotlivých `log.*` volání:

1. **Logy → OTLP:** `newLogHandler` (`cmd/logger.go`) je jediné místo, kde vzniká slog handler. Obalí se mostem `otelslog` (nebo fan-out handlerem, který loguje lokálně i exportuje přes OTLP) — žádné call site se nemění.
2. **Korelace → `span_id`:** `LogAttrs(ctx)` je jediný zdroj korelačních atributů. Přidání `span_id` (z OTel `SpanContext` v ctx) je změna jedné funkce.
3. **Traces:** `otelhttp` na HTTP serveru (span per request) + span v bus middleware per command — přesně tam, kde se dnes měří `duration_ms`. `trace_id` se sjednotí s OTel trace id, takže logy a traces se v Grafaně propojí.
4. **Backendy:** logy → Loki, traces → Tempo, metriky → Prometheus/Mimir. Lokálně vše naráz přes image `grafana/otel-lgtm` (OTLP na portech 4317/4318).

Zbývající kroky (Sentry, OTel traces) sleduje [Roadmap](/roadmap), Fáze 5.
