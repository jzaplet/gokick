package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"gokick/app/domain/shared"

	"github.com/google/uuid"
)

// drainAudit persists the audit events a run handler recorded — the worker's
// analogue of the bus AuditMiddleware. Actor/IP are nil: a background run has no
// principal. Best-effort like the bus (a failed write is logged, never fails the
// run) and a no-op when the worker has no AuditLogger (buildRunContext then
// installed no collector, so Flush is empty anyway — the nil guard is explicit).
// Cancellation is detached so a shutdown mid-drain still persists the trail.
func (w *RunWorker) drainAudit(ctx context.Context, log *slog.Logger, runCtx context.Context) {
	if w.auditLogger == nil {
		return
	}
	events := shared.AuditCollectorFromContext(runCtx).Flush()
	if len(events) == 0 {
		return
	}
	flushCtx := context.WithoutCancel(ctx)
	now := time.Now()
	for _, evt := range events {
		rec, err := buildAuditRecord(evt, now)
		if err == nil {
			err = w.auditLogger.Save(flushCtx, rec)
		}
		if err != nil {
			log.LogAttrs(flushCtx, slog.LevelError, "run worker: audit write failed",
				slog.String(logKeyAuditAction, evt.Action),
				slog.Any(shared.LogKeyError, err))
		}
	}
}

// buildAuditRecord maps a handler-emitted event to its persistence record with no
// principal (background run: actor/IP nil). Mirrors the bus middleware's writeRecord
// field mapping; kept worker-local so the security-critical middleware path stays
// untouched. Marshaling the metadata is the only failure mode.
func buildAuditRecord(evt shared.AuditEvent, now time.Time) (*shared.AuditRecord, error) {
	rec := &shared.AuditRecord{
		ID:        uuid.NewString(),
		Action:    evt.Action,
		CreatedAt: now,
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
			return nil, err
		}
		rec.Metadata = b
	}
	return rec, nil
}
