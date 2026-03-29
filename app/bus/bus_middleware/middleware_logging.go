package bus_middleware

import (
	"context"
	"log/slog"
	"myapp/app/bus"
	"myapp/app/http/http_middleware"
	"time"
)

func LoggingMiddleware(logger *slog.Logger) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		traceID := http_middleware.TraceIDFromContext(ctx)
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
