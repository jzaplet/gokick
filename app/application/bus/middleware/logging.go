package middleware

import (
	"context"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"log/slog"
	"time"
)

// Bus-middleware-local structured-log keys. Cross-cutting keys (trace_id,
// user_id, command, duration_ms, error, event) live in shared.LogKey*; these
// are specific to this package's log lines. sloglint's no-raw-keys forbids
// bare string keys, so every key is a constant.
const (
	logKeySourceCommand = "source_command"
	logKeyAction        = "action"
	logKeyPanic         = "panic"
	logKeyStack         = "stack"
)

func LoggingMiddleware(logger *slog.Logger) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		cmdAttr := slog.String(shared.LogKeyCommand, name)
		logger.LogAttrs(ctx, slog.LevelInfo, "bus: executing",
			append(shared.LogAttrs(ctx), cmdAttr)...)

		start := time.Now()
		result, err := next(ctx)
		durAttr := shared.DurationMsAttr(time.Since(start))

		if err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"bus: failed",
				append(
					shared.LogAttrs(ctx),
					cmdAttr,
					durAttr,
					slog.Any(shared.LogKeyError, err),
				)...)
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "bus: completed",
				append(shared.LogAttrs(ctx), cmdAttr, durAttr)...)
		}

		return result, err
	}
}
