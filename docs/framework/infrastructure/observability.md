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

Pravidlo: standardizovanou korelační/měřící slovní zásobu produkují helpery a konstanty — `shared.LogAttrs` (`trace_id` + `user_id`), `shared.DurationMsAttr` (`duration_ms`) a konstanty `shared.LogKey*` — používané hlavně v **bus middleware**, kde na konzistenci záleží nejvíc. Leaf komponenty (worker, scheduler, server, CLI), všudypřítomný klíč `error` a jednorázové komponentní klíče (`addr`, `slot`, `name`, `nickname`) mohou používat prosté literály.


## Korelace: `LogAttrs(ctx)`

`shared.LogAttrs(ctx) []slog.Attr` je **jediný** zdroj korelačních atributů — vrátí `trace_id` (když je) a `user_id` (u autentizovaných requestů). Skládá se přímo s metodou `logger.LogAttrs`, takže není potřeba žádná `[]any` konverze:

```go
attrs := append(shared.LogAttrs(ctx), slog.String(shared.LogKeyCommand, name))
logger.LogAttrs(ctx, slog.LevelInfo, "bus: completed",
    append(attrs, shared.DurationMsAttr(d))...)
```

- `user_id` je dostupné na **bus vrstvě** — claims injektuje HTTP `AuthMiddleware` ještě před voláním busu. Pro login/refresh (neautentizované, `SkipPermission`) se `user_id` vynechá.
- Globální HTTP `LoggingMiddleware` běží **před** auth → nese `trace_id`, ne `user_id`. To je v pořádku — spolehlivá vrstva pro `user_id` je bus.
- Dobu vždy loguj přes `shared.DurationMsAttr(d)` — číselné `duration_ms`, ne `time.Duration` (které se v JSON serializuje jako nanosekundy).


## Šev pro OpenTelemetry

Systém je připravený tak, aby OTel šel doplnit **lokalizovaně**, bez zásahu do jednotlivých `log.*` volání:

1. **Logy → OTLP:** `newLogHandler` (`cmd/logger.go`) je jediné místo, kde vzniká slog handler. Obalí se mostem `otelslog` (nebo fan-out handlerem, který loguje lokálně i exportuje přes OTLP) — žádné call site se nemění.
2. **Korelace → `span_id`:** `LogAttrs(ctx)` je jediný zdroj korelačních atributů. Přidání `span_id` (z OTel `SpanContext` v ctx) je změna jedné funkce.
3. **Traces:** `otelhttp` na HTTP serveru (span per request) + span v bus middleware per command — přesně tam, kde se dnes měří `duration_ms`. `trace_id` se sjednotí s OTel trace id, takže logy a traces se v Grafaně propojí.
4. **Backendy:** logy → Loki, traces → Tempo, metriky → Prometheus/Mimir. Lokálně vše naráz přes image `grafana/otel-lgtm` (OTLP na portech 4317/4318).

Zbývající kroky (Sentry, OTel traces) sleduje [Roadmap](/roadmap), Fáze 5.
