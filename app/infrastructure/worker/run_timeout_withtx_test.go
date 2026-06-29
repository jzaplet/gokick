package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func countRunsByKind(t *testing.T, fx *testfx.Fixture, kind string) int {
	t.Helper()
	var n int
	if err := fx.DB.DB().GetContext(context.Background(), &n,
		`SELECT COUNT(*) FROM runs WHERE kind = ?`, kind); err != nil {
		t.Fatalf("count runs %q: %v", kind, err)
	}
	return n
}

// A per-attempt timeout: a handler that blows its deadline is a RETRYABLE failure
// (rescheduled with backoff), not a completion — even though it returns ctx.Err().
func TestRunWorker_Timeout_ReschedulesAsFailure(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_timeout.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		<-ctx.Done() // honor the deadline
		return ctx.Err()
	}
	reg, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{
		"slow": runapp.FireAndForget(handler, 100*time.Millisecond),
	}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, nil, fastCfg())
	r := enqueueRunW(t, fx, "slow", 1) // 1 retry available

	stop := startWorker(w)
	defer stop()
	waitFor(t, "timed-out run rescheduled (not terminal)", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Attempts == 1
	})
	if g := findW(t, fx, r.ID); g.CompletedAt != nil || g.FailedAt != nil {
		t.Fatal("a timed-out run within its retry budget must reschedule, not be terminal")
	}
}

// A handler can make several writes atomically via shared.WithTx (short transaction
// it scopes itself). On a clean return the writes commit.
func TestRunWorker_HandlerWithTx_CommitsChildWrite(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_withtx_commit.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return shared.WithTx(ctx, func(txCtx context.Context) error {
			return fx.Runs.Enqueue(txCtx, run.NewRun("withtx.child", []byte(`{}`), 0))
		})
	}
	reg, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{
		"withtx.parent": {Handler: handler},
		"withtx.child": {
			Handler: func(context.Context, *run.Run, runapp.Checkpointer) error { return nil },
		},
	}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	// transactor = fx.DB (implements shared.Transactor) → WithTx works.
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, fx.DB, fastCfg())
	enqueueRunW(t, fx, "withtx.parent", 0)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "child row committed via WithTx", func() bool {
		return countRunsByKind(t, fx, "withtx.child") == 1
	})
}

// If the WithTx block returns an error, the whole short transaction rolls back —
// the writes it made do not persist (all-or-nothing).
func TestRunWorker_HandlerWithTx_RollsBackOnError(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_withtx_rollback.db")
	boom := errors.New("fail after the write")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return shared.WithTx(ctx, func(txCtx context.Context) error {
			if e := fx.Runs.Enqueue(txCtx, run.NewRun("withtx.rbchild", []byte(`{}`), 0)); e != nil {
				return e
			}
			return boom // roll the child enqueue back with the tx
		})
	}
	reg, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{
		"withtx.rbparent": {Handler: handler},
	}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, fx.DB, fastCfg())
	r := enqueueRunW(t, fx, "withtx.rbparent", 0) // run-once → first failure is terminal

	stop := startWorker(w)
	defer stop()
	waitFor(t, "parent failed (handler returned the WithTx error)", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.FailedAt != nil
	})
	if n := countRunsByKind(t, fx, "withtx.rbchild"); n != 0 {
		t.Fatalf("WithTx error must roll back the child enqueue, found %d rows", n)
	}
}
