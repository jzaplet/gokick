---
layout: 'page'
uri: '/framework/infrastructure/observability'
position: 6
slug: 'framework-infrastructure-observability'
parent: 'framework-infrastructure'
navTitle: 'Observability'
title: 'Observability'
description: 'Trace ID, structured logging, Sentry, OpenTelemetry.'
---

# Observability

Třívrstvý observability stack: trace ID propagace, structured logging přes `log/slog`, Sentry pro error tracking. OpenTelemetry jako volitelné rozšíření.


## Trace ID

Každý HTTP request dostane unikátní trace ID. Propaguje se celým systémem přes `context.Context`.


### Middleware

```go
// middleware/trace.go

func TraceMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            traceID := r.Header.Get("X-Trace-Id")
            if traceID == "" {
                traceID = uuid.New().String()
            }

            ctx := context.WithValue(r.Context(), traceIDKey, traceID)
            w.Header().Set("X-Trace-Id", traceID)

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func TraceIDFromContext(ctx context.Context) string {
    id, _ := ctx.Value(traceIDKey).(string)
    return id
}
```


### HTTP middleware chain

Trace middleware je **první** v chain – všechny následující middleware a handlery mají trace ID k dispozici:

```
Request → Trace → CORS → Logging → JWT Auth → Handler
```


### Response header

Klient (frontend, curl, monitoring) vždy obdrží `X-Trace-Id` v response. Umožňuje korelaci requestu s logy a error reporty.


## Structured logging (slog)

`log/slog` s trace ID ve všech log záznamech.


### Logger setup

```go
// di_container/ nebo main.go

logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
slog.SetDefault(logger)
```


### Trace-aware logging

Bus `LoggingMiddleware` automaticky přidává trace ID do každého logu:

```go
// bus/middleware_logging.go

func LoggingMiddleware(logger *slog.Logger) Middleware {
    return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
        traceID := middleware.TraceIDFromContext(ctx)
        log := logger.With("trace_id", traceID, "command", name)

        log.Info("bus: executing")
        start := time.Now()

        result, err := next(ctx)

        duration := time.Since(start)
        if err != nil {
            log.Error("bus: failed", "duration", duration, "error", err)
        } else {
            log.Info("bus: completed", "duration", duration)
        }
        return result, err
    }
}
```


### HTTP logging middleware

```go
// middleware/logging.go

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            traceID := TraceIDFromContext(r.Context())
            logger.Info("http: request",
                "trace_id", traceID,
                "method", r.Method,
                "path", r.URL.Path,
            )
            next.ServeHTTP(w, r)
        })
    }
}
```


### Log output

```json
{
  "time": "2026-03-29T10:00:00Z",
  "level": "INFO",
  "msg": "bus: executing",
  "trace_id": "550e8400-e29b-41d4-a716-446655440000",
  "command": "CreateUser"
}
```


## Sentry

Error tracking s automatickým trace ID tagováním.


### Inicializace

```go
// main.go nebo di_container/

import "github.com/getsentry/sentry-go"

sentry.Init(sentry.ClientOptions{
    Dsn:              os.Getenv("SENTRY_DSN"),
    TracesSampleRate: 0.1,
    Environment:      os.Getenv("APP_ENV"),
})
defer sentry.Flush(2 * time.Second)
```


### Bus middleware

Rozšíření `RecoveryMiddleware` o Sentry reporting:

```go
// bus/middleware_recovery.go

func RecoveryMiddleware(logger *slog.Logger) Middleware {
    return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
        defer func() {
            if r := recover(); r != nil {
                traceID := middleware.TraceIDFromContext(ctx)
                logger.Error("bus: panic", "command", name, "trace_id", traceID, "panic", r)

                sentry.WithScope(func(scope *sentry.Scope) {
                    scope.SetTag("trace_id", traceID)
                    scope.SetTag("command", name)
                    sentry.CurrentHub().Recover(r)
                })
            }
        }()
        return next(ctx)
    }
}
```


### HTTP error reporting

`response.HandleError` reportuje 500 chyby do Sentry:

```go
// response/json.go

func HandleError(w http.ResponseWriter, err error) {
    var httpErr HTTPError
    if errors.As(err, &httpErr) {
        Error(w, httpErr.HTTPStatus(), err)
    } else {
        // 500 → report do Sentry
        sentry.CaptureException(err)
        Error(w, http.StatusInternalServerError, err)
    }
}
```


### Konfigurace

```env
SENTRY_DSN=https://xxx@sentry.io/123
APP_ENV=production
```

Sentry se aktivuje jen pokud je `SENTRY_DSN` nastavený. V development prostředí bez DSN se chyby jen logují.


## OpenTelemetry (volitelné)

Pro distributed tracing přes více služeb. Pro single-binary SQLite aplikaci **není nutné** – trace ID + slog + Sentry pokrývají potřeby.

Pokud bude potřeba (microservices, external API calls):

```go
import "go.opentelemetry.io/otel"

// Exporter do Grafana Tempo / Jaeger
exporter, _ := otlptrace.New(ctx, otlptracegrpc.NewClient())
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
otel.SetTracerProvider(tp)
```

Bus middleware by vytvářel span per command/query. HTTP middleware by propagoval W3C trace context headers.


## Dependency impact

| Balíček | Nová závislost | Popis |
|---|---|---|
| `middleware/` | žádná (stdlib) | Trace ID generování + propagace |
| `bus/` | žádná | Trace ID z contextu do logů |
| `response/` | `sentry-go` (volitelné) | Error reporting |
| `main.go` | `sentry-go` (volitelné) | Inicializace |

Trace ID middleware a slog jsou **zero-dependency** – jen stdlib. Sentry je volitelná dependency aktivovaná přes env var.
