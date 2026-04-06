package middleware

import (
	"context"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"log/slog"
)

func DispatchEventsMiddleware(
	logger *slog.Logger,
	collector *shared.EventCollector,
	eventBus *bus.EventBus,
) bus.Middleware {
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
			eventBus.Dispatch(ctx, event)
		}

		return result, nil
	}
}
