---
name: gk-logging
description: Strukturované logování přes jedinou slog cestu — konstantní klíče (LogKey*), korelace requestů přes trace_id/user_id a statické vynucení lintem (depguard/forbidigo/sloglint). Use when přidáváš log řádek, hledáš „odkud se loguje" / „proč mi lint zařval na fmt.Println nebo slog.New", nebo nastavuješ formát/level přes APP_LOG_*.
layout: 'page'
uri: '/skills/gk-logging'
position: 10
slug: 'skills-gk-logging'
parent: 'skills-observability'
navTitle: 'gk-logging'
title: 'GK — Structured logging'
---

# GK — Structured logging

Všechno logování teče jednou cestou: injektovaný `*slog.Logger` postavený na jediném místě, klíče jsou konstanty a lint to staticky hlídá.

## What & when

- Sáhni sem, když **přidáváš log řádek** a nevíš, jak (kde vzít logger, jaký klíč použít, jak připojit `trace_id`/`user_id`).
- Když ti **lint zařval** na `fmt.Println`, `slog.New`, `os.Stdout`, syrový string klíč, mixování argumentů — vysvětlení je tady.
- Když řešíš **formát/úroveň logů** lokálně vs. v produkci (`APP_LOG_FORMAT`, `APP_LOG_LEVEL`).
- NEtýká se hlášení **neočekávaných** chyb (paniky, terminální selhání jobu) do Sentry — to je error reporting, samostatná cesta (viz `Related`). Běžné logování není Sentry.

## For non-tech / juniors

Log je „deníček" aplikace — řádky typu „přišel request", „command proběhl za 3 ms", „login selhal". **Strukturovaný** log znamená, že řádek není jen věta, ale i sada **pojmenovaných polí** (`trace_id=…`, `user_id=…`, `duration_ms=3`). Pole jdou pak strojově filtrovat ve sběrači logů (např. Grafana/Loki) — „ukaž mi vše s tímhle `trace_id`" tě provede jedním requestem napříč vrstvami.

Aby ta pole byla po celé aplikaci **stejně pojmenovaná** (ne `user_id` tady a `userId` jinde), jsou jména klíčů **konstanty** v jednom souboru. A aby nikdo omylem nelogoval bokem (do konzole, do souboru, přes cizí knihovnu) a log se neztratil, **linter** všechny ostatní cesty zakazuje. Je to záměrně **jedna cesta ven** — snadno se na ni jednou připne třeba export do dalšího systému, bez přepisování stovek míst.

## How it works

**Jediný konstruktor.** `*slog.Logger` vzniká **jen** v `cmd/logger.go` (`newLogger` → `newLogHandler`) a všude jinde se **injektuje** přes Wire DI. Bootstrap je v `cmd/main.go`: nejdřív `config.LoadStartup()` (načte `.env`), pak `newLogger(startup.LogFormat, parseLogLevel(startup.LogLevel), sentryEnabled)`. Píše se na **stderr**.

- **Formát** — `APP_LOG_FORMAT`: `json` (default, pro agregátory) nebo `text` (čitelné pro lokální `make serve`). Cokoli jiného degraduje na `json` (`newLogHandler`).
- **Level** — `APP_LOG_LEVEL`: `debug` / `info` (default) / `warn` / `error`. Neznámá hodnota → `info` (`parseLogLevel`).

**Konstantní klíče.** `app/domain/shared/log.go` definuje cross-cutting klíče jako konstanty — `LogKeyTraceID` (`trace_id`), `LogKeyUserID` (`user_id`), `LogKeyCommand` (`command`), `LogKeyDurationMs` (`duration_ms`), `LogKeyError`, `LogKeyEvent`, `LogKeyJobKind`, `LogKeyRetryInMs` + HTTP set (`LogKeyMethod`, `LogKeyPath`, `LogKeyURL`, `LogKeyUserAgent`, …). Klíč použitý jen na jednom místě zůstává **package-local** konstanta `logKey*` (např. `logKeySourceCommand`, `logKeyAction`, `logKeyPanic` v `app/application/bus/middleware/logging.go`).

**Korelace.** `shared.LogAttrs(ctx) []slog.Attr` je **jediný** zdroj korelačních atributů — vrátí `trace_id` (z `TraceMiddleware`, když je) a `user_id` (z `AuthClaims` v ctx, u autentizovaných requestů). Skládá se přímo s metodou `logger.LogAttrs` (bere `[]slog.Attr` nativně, žádná `[]any` konverze). Dobu trvání loguj přes `shared.DurationMsAttr(d)` — číselné `duration_ms` (ne `time.Duration`, ta by se v JSON serializovala jako nanosekundy).

Kanonický vzor (z `app/application/bus/middleware/logging.go`):

```go
cmdAttr := slog.String(shared.LogKeyCommand, name)
logger.LogAttrs(ctx, slog.LevelInfo, "bus: executing",
    append(shared.LogAttrs(ctx), cmdAttr)...)
// …po doběhnutí:
durAttr := shared.DurationMsAttr(time.Since(start))
logger.LogAttrs(ctx, slog.LevelInfo, "bus: completed",
    append(shared.LogAttrs(ctx), cmdAttr, durAttr)...)
```

**Statické vynucení** (`.golangci.yml`) — drží jedinou cestu:

- **`depguard`** — import **allow-list** (ne deny): povolen jen `$gostd`, `gokick` a vyjmenované přímé závislosti. Cizí logger (charm/log, hclog, glog, …) padá už importem. Nová závislost = nový řádek v allow-listu. (`sentry-go` je v allow vědomě — error sink, ne logger.)
- **`forbidigo`** — zakázáno: `fmt.Print*` + `print`/`println` (stdout), stdlib `log.*`, `slog.New*` mimo `cmd/`, `slog.Default()`, `os.Stdout`/`os.Stderr`, `os.Create`/`os.OpenFile`/`os.WriteFile`/`os.NewFile`, `syscall.Write` (= logování do souboru).
- **`sloglint`** — `no-global` (žádný globální default logger), `static-msg` (zpráva konstantní), `no-raw-keys` (každý klíč konstanta), `key-naming-case: snake`, `no-mixed-args` (nemíchat kv páry a `slog.Attr`).
- **Úzké výjimky** (`exclusions`): `presentation/console/` smí `fmt.Print` (CLI výstup), `cmd/` smí `slog.New` + `os.Stderr` (konstruktor) + je mimo sloglint (bootstrap), `app/domain/shared/log.go` definuje klíče (mimo no-raw-keys), `app/internal/testfx/` a `*_test.go` jsou vyňaté.

## Recipe: přidat log řádek

1. **Měj injektovaný `*slog.Logger`** ve struktuře (přes Wire DI) — nikdy si ho nestav přes `slog.New`, nikdy nesahej na `slog.Default()`.
2. **Vyber klíče.** Cross-cutting → `shared.LogKey*`. Jednorázový komponentní klíč → přidej package-local konstantu `logKey<Name>` (snake_case hodnota).
3. **Loguj ctx-formou** `logger.LogAttrs(ctx, level, "konstantní zpráva", attrs...)`, kde `attrs` začni `shared.LogAttrs(ctx)` (přitáhne `trace_id`/`user_id`). Dobu přidej `shared.DurationMsAttr(d)`.
4. **Zprávu drž jako konstantní string** (`static-msg`) — proměnné dej do atributů, ne do zprávy. Nemíchej `slog.Attr` a `key, value` páry v jednom volání (`no-mixed-args`).
5. **`make lint`** — `golangci-lint` ověří depguard/forbidigo/sloglint.

## Invariants & pitfalls

- **Jedna logovací cesta.** Logger se staví **jen** v `cmd/logger.go`; všude jinde injektovaný `*slog.Logger`. Žádný `fmt.Print*`, stdlib `log`, cizí logger, `slog.New` mimo `cmd/`, `slog.Default()`, zápis na `os.Stdout/Stderr` ani do souboru.
- **Každý klíč je konstanta** (`no-raw-keys`), `snake_case`. Holý string literál jako klíč lint odmítne.
- **Zprávy jsou konstantní** (`static-msg`), argumenty se nemíchají (`no-mixed-args`).
- **Korelaci ber z `shared.LogAttrs(ctx)`**, dobu z `shared.DurationMsAttr(d)` — neskládej `trace_id`/`user_id`/`duration_ms` ručně. `LogAttrs` alokuje čerstvý slice, takže do výsledku můžeš bezpečně `append`.
- **`user_id` je spolehlivé až na bus vrstvě** (claims injektuje `AuthMiddleware` před busem). Globální HTTP access log běží **před** auth → nese `trace_id`, ne `user_id`. Login/refresh (`SkipPermission`, neautentizované) `user_id` nemají — to je v pořádku.
- **Logování ≠ error reporting.** Běžné návratové chyby (validace, auth, 4xx) se **nehlásí** do Sentry, jen recovery/terminal cesty — jinak tracker utone v šumu.
- **Nová závislost = řádek v depguard allow-listu** (`.golangci.yml`), jinak import neprojde lintem.
- **OTel zatím není** — je jen připravený šev (`newLogHandler` / `LogAttrs`), nikoli hotová funkce. Nepiš logy „pro OTel", piš je standardní cestou.

## Related

- `/gk-bus` — middleware chain loguje executing/completed/failed přes tenhle vzor (`shared.LogAttrs` + `command` + `duration_ms`).
- `/gk-config` — `APP_LOG_FORMAT` / `APP_LOG_LEVEL` a `config.LoadStartup` (načtení `.env` před stavbou loggeru).
- `/gk-architecture` — proč logger žije v `cmd/` a injektuje se přes DI.
- Kód: konstruktor `cmd/logger.go` + bootstrap `cmd/main.go`; klíče a korelace `app/domain/shared/log.go`; lint `/.golangci.yml`; reálný call site `app/application/bus/middleware/logging.go`.
