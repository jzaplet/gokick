package middleware

import (
	"context"
	"log/slog"
	"myapp/app/application/bus"
	"myapp/app/domain/shared"
)

func DispatchEventsMiddleware(logger *slog.Logger, collector *shared.EventCollector) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		result, err := next(ctx)
		if err != nil {
			return result, err
		}

		events := collector.Flush()
		for _, event := range events {
			traceID := shared.TraceIDFromContext(ctx)
			logger.Info("bus: event dispatched",
				"trace_id", traceID,
				"event", event.EventName(),
				"source_command", name,
			)
			// TODO: dispatch to registered event handlers via EventBus
		}

		return result, nil
	}
}
