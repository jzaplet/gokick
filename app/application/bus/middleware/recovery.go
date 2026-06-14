package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

func RecoveryMiddleware(logger *slog.Logger, reporter shared.ErrorReporter) bus.Middleware {
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
				// Wrap as a PanicError so the tracker labels it "panic" rather
				// than the generic *errors.errorString from fmt.Errorf. The
				// returned err maps to a 500 either way (default error mapping).
				err = &shared.PanicError{
					Value:   r,
					Message: fmt.Sprintf("bus: panic in %s: %v", name, r),
				}
				// Report the recovered panic to the error tracker (no-op without
				// a DSN). Only panics reach here — ordinary command errors do not.
				reporter.Capture(ctx, err, slog.String(shared.LogKeyCommand, name))
			}
		}()
		return next(ctx)
	}
}
