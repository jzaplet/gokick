package middleware

import (
	"context"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"log/slog"
)

// DispatchEventsMiddleware must wrap TransactionMiddleware so that a failed
// commit also drops collected events.
func DispatchEventsMiddleware(
	logger *slog.Logger,
	eventBus *bus.EventBus,
) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		ctxWithCollector, collector := shared.ContextWithEventCollector(ctx)

		result, err := next(ctxWithCollector)
		if err != nil {
			return result, err
		}

		// Event handlers must not Collect — Flush runs once.
		for _, event := range collector.Flush() {
			logger.LogAttrs(ctxWithCollector, slog.LevelInfo, "bus: event dispatched",
				append(shared.LogAttrs(ctxWithCollector),
					slog.String(shared.LogKeyEvent, event.EventName()),
					slog.String(logKeySourceCommand, name),
				)...)
			eventBus.Dispatch(ctxWithCollector, event)
		}

		return result, nil
	}
}
