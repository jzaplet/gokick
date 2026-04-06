package middleware

import (
	"context"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"log/slog"
	"time"
)

func LoggingMiddleware(logger *slog.Logger) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		traceID := shared.TraceIDFromContext(ctx)
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
