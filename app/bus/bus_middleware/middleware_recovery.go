package bus_middleware

import (
	"context"
	"fmt"
	"log/slog"
	"myapp/app/bus"
)

func RecoveryMiddleware(logger *slog.Logger) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (result any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("bus: panic recovered", "command", name, "panic", r)
				err = fmt.Errorf("bus: panic in %s: %v", name, r)
			}
		}()
		return next(ctx)
	}
}
