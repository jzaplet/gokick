package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"gokick/app/application/bus"
)

func RecoveryMiddleware(logger *slog.Logger) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (result any, err error) {
		defer func() {
			if r := recover(); r != nil {
				// Capture the stack at the point of recovery so the log shows
				// where the panic originated, not just its value.
				logger.Error("bus: panic recovered",
					"command", name,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = fmt.Errorf("bus: panic in %s: %v", name, r)
			}
		}()
		return next(ctx)
	}
}
