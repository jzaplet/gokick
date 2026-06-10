package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

func RecoveryMiddleware(logger *slog.Logger) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (result any, err error) {
		defer func() {
			if r := recover(); r != nil {
				// Capture the stack at the point of recovery so the log shows
				// where the panic originated, not just its value.
				logger.LogAttrs(ctx, slog.LevelError, "bus: panic recovered",
					append(shared.LogAttrs(ctx),
						slog.String(shared.LogKeyCommand, name),
						slog.Any(logKeyPanic, r),
						slog.String(logKeyStack, string(debug.Stack())),
					)...)
				err = fmt.Errorf("bus: panic in %s: %v", name, r)
			}
		}()
		return next(ctx)
	}
}
