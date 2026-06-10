package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"

	"github.com/google/uuid"
)

// AuditMiddleware drains an AuditCollector after the handler runs and
// persists each event through AuditLogger. It MUST live outside the
// transaction middleware so:
//
//  1. Audit writes don't roll back together with business state.
//  2. Failure-mode events (login_failed, permission_denied) still get
//     persisted even though the business handler returned an error.
//
// Audit write failures are logged but never propagated to the caller —
// the security trail is best-effort by design; a degraded trail must
// not turn a normal command into a 500.
func AuditMiddleware(logger *slog.Logger, audit shared.AuditLogger) bus.Middleware {
	return func(
		ctx context.Context,
		name string,
		cmd any,
		next func(ctx context.Context) (any, error),
	) (any, error) {
		collector := &shared.AuditCollector{}
		ctxWithCollector := shared.ContextWithAuditCollector(ctx, collector)

		result, handlerErr := next(ctxWithCollector)

		events := collector.Drain()
		if len(events) == 0 {
			return result, handlerErr
		}

		// Detach cancellation: a client that disconnected mid-request
		// shouldn't be able to abort the audit write. Timestamp once here
		// (not per event, not in the handler) so all events from one command
		// share the wall clock the middleware saw.
		flushCtx := context.WithoutCancel(ctxWithCollector)
		now := time.Now()
		for _, evt := range events {
			if err := writeRecord(flushCtx, audit, evt, now); err != nil {
				logger.LogAttrs(flushCtx, slog.LevelError, "audit: write failed",
					append(shared.LogAttrs(flushCtx),
						slog.String(logKeyAction, evt.Action),
						slog.String(shared.LogKeyCommand, name),
						slog.Any(shared.LogKeyError, err),
					)...)
			}
		}

		return result, handlerErr
	}
}

func writeRecord(
	ctx context.Context,
	audit shared.AuditLogger,
	evt shared.AuditEvent,
	now time.Time,
) error {
	rec := &shared.AuditRecord{
		ID:        uuid.New().String(),
		Action:    evt.Action,
		CreatedAt: now,
	}

	if claims := shared.ClaimsFromContext(ctx); claims != nil && claims.UserID != "" {
		v := claims.UserID
		rec.ActorUserID = &v
	}
	if ip := shared.ActorIPFromContext(ctx); ip != "" {
		v := ip
		rec.ActorIP = &v
	}
	if evt.TargetType != "" {
		v := evt.TargetType
		rec.TargetType = &v
	}
	if evt.TargetID != "" {
		v := evt.TargetID
		rec.TargetID = &v
	}
	if len(evt.Metadata) > 0 {
		b, err := json.Marshal(evt.Metadata)
		if err != nil {
			return err
		}
		rec.Metadata = b
	}

	return audit.Save(ctx, rec)
}
