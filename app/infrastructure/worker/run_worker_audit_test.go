package worker

import (
	"context"
	"testing"
	"time"

	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// F-004: a run handler that records an audit event must have it persisted. The
// worker installs a real AuditCollector into the run ctx and drains it to the
// AuditLogger after the handler returns — its analogue of the bus AuditMiddleware,
// so a background run gets the SAME complete middleware set instead of the
// throwaway collector that silently dropped Record. The drain runs before finalize
// stamps completed_at, so once the run reads completed the row must already be
// there; without the drain it never lands and this fails.
func TestRunWorker_HandlerAuditEventIsPersisted(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_audit.db")

	handler := func(ctx context.Context, r *run.Run, _ runapp.Checkpointer) error {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
			Action:     "run.did.thing",
			TargetType: "run",
			TargetID:   r.ID,
			Metadata:   map[string]any{"k": "v"},
		})
		return nil
	}

	reg, err := runapp.NewHandlerRegistry(
		map[string]runapp.Registration{"audited": {Handler: handler}}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	w := NewRunWorker(
		silentLogger(), &countingReporter{}, fx.Runs, reg,
		nil, nil, fx.NewAuditLogger(), fastCfg(),
	)

	r := enqueueRunW(t, fx, "audited", 0)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "run completed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CompletedAt != nil
	})

	// The drain runs before completed_at is stamped, so the row is already written.
	var n int
	if e := fx.DB.DB().GetContext(context.Background(), &n,
		`SELECT COUNT(*) FROM audit_log WHERE action='run.did.thing' AND target_id=?`, r.ID); e != nil {
		t.Fatalf("count audit rows: %v", e)
	}
	if n != 1 {
		t.Fatalf("run handler's audit event must be persisted by the worker drain, got %d rows", n)
	}
}
