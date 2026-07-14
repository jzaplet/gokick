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
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, nil, nil, fastCfg())
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
			child, _ := run.NewRun("withtx.child", []byte(`{}`), 0)
			return fx.Runs.Enqueue(txCtx, child)
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
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, fx.DB, nil, fastCfg())
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
			rbchild, _ := run.NewRun("withtx.rbchild", []byte(`{}`), 0)
			if e := fx.Runs.Enqueue(txCtx, rbchild); e != nil {
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
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, fx.DB, nil, fastCfg())
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

// timedOut is ranked ABOVE success in finalize: a handler that returns NIL exactly as
// its deadline fires must still be treated as a (retryable) timeout, not Completed.
// The sibling test above uses a handler returning ctx.Err(), which would route to
// handleFailure via the `default` case even if the timedOut flag were broken — this
// one returns nil, so only correct timedOut detection + ranking keeps it off Complete.
func TestRunWorker_Timeout_NilReturn_NotCompleted(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_timeout_nil.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		<-ctx.Done() // deadline fires
		return nil   // clean-stop nil, NOT ctx.Err()
	}
	reg, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{
		"slownil": runapp.FireAndForget(handler, 100*time.Millisecond),
	}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, nil, nil, fastCfg())
	r := enqueueRunW(t, fx, "slownil", 1)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "timed-out nil-return rescheduled (not completed)", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Attempts == 1
	})
	if g := findW(t, fx, r.ID); g.CompletedAt != nil {
		t.Fatal(
			"a nil return AT the deadline must be treated as timed-out (retryable), not completed",
		)
	}
}

// Backstop: a NON-ctx-aware handler that blows its per-attempt timeout must not pin
// the lease forever. After the deadline the heartbeat keeps renewing only for the
// grace window, then ABANDONS — the lease lapses so another worker can reclaim (and
// the poison cap eventually terminates a persistent offender). Uses a short per-kind
// lease so the grace (cancelGraceLeaseMultiple*lease) is small. The handler ignores
// ctx on purpose and leaks until release is closed at test end.
func TestRunWorker_Timeout_NonCtxAwareHandler_AbandonsForReclaim(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_timeout_stuck.db")
	release := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		<-release // IGNORES ctx — only the test can free it
		return nil
	}
	reg, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{
		// short lease → small grace; timeout << lease so it fires well before the lease.
		"stuck": runapp.Durable(handler, 120*time.Millisecond, 40*time.Millisecond),
	}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	cfg := fastCfg()
	cfg.MaxInFlight = 1 // the stuck handler pins the only slot, so the worker can't reclaim
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, nil, nil, nil, cfg)
	r := enqueueRunW(t, fx, "stuck", 0)

	stop := startWorker(w)
	defer stop()
	defer close(release) // free the stuck handler so the worker can drain (LIFO: before stop)

	// The heartbeat abandons after the grace window; once it stops renewing, the lease
	// lapses (locked_until falls into the past) — the run is reclaimable, not pinned.
	waitFor(t, "timed-out non-ctx-aware run abandoned (lease lapses)", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CompletedAt == nil && g.FailedAt == nil &&
			g.LockedUntil != nil && g.LockedUntil.Before(time.Now())
	})
}
