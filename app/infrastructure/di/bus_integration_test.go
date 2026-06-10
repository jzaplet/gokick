package di

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"gokick/app/application/bus"
	jobapp "gokick/app/application/job"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/security"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	"gokick/app/internal/testfx"
)

// skipPermCmd opts out of permission checks (so AuthorizeMiddleware lets it
// through) but is NOT SkipsTransaction — we WANT the bus tx to wrap it.
type skipPermCmd struct{}

func (skipPermCmd) SkipPermissionCheck() {}

type noopDispatcher struct{}

func (noopDispatcher) Enqueue(context.Context, string, int, any, ...shared.EnqueueOption) error {
	return nil
}

// newProductionCommandBus builds the CommandBus through the SAME provider the
// binary uses (provideCommandBus), so the middleware chain under test cannot
// drift from production the way a hand-assembled chain (or testfx.NewBuses,
// which omits Audit + JobDispatcher) silently can.
func newProductionCommandBus(
	t *testing.T,
	fx *testfx.Fixture,
	dispatcher shared.JobDispatcher,
) *bus.CommandBus {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := security.NewPermissionChecker()
	eventBus := provideEventBus(logger, provideEventHandlers(), shared.NopReporter{})
	audit := sqliteaudit.NewRepository(fx.DB)
	return provideCommandBus(
		logger,
		fx.DB,
		checker,
		eventBus,
		dispatcher,
		audit,
		shared.NopReporter{},
	)
}

// audit.md guarantee #1: a security-relevant event recorded by a handler MUST
// survive the rollback of that handler's business transaction. AuditMiddleware
// sits OUTSIDE TransactionMiddleware (verified here through the real provider
// chain), and the audit repo writes on the raw pool — so the failed_login-style
// trail persists even when the business write is undone. Closes the otherwise
// untested app-events-audit-26/27.
func TestCommandBus_AuditSurvivesBusinessRollback(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit_rollback.db"))
	u := fx.SeedUser(t, "victim", "password123", "user")

	cmdBus := newProductionCommandBus(t, fx, noopDispatcher{})

	err := bus.ExecVoid(
		ctx,
		cmdBus.Bus,
		"TamperUser",
		skipPermCmd{},
		func(ctx context.Context) error {
			// Business write that joins the bus tx (Update uses r.Conn(ctx)).
			u.Nickname = "tampered"
			if e := fx.Users.Update(ctx, u); e != nil {
				return e
			}
			// Security-relevant audit event.
			shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
				Action:     "user.role_changed",
				TargetType: "user",
				TargetID:   u.ID,
				Metadata:   map[string]any{"new_role": "admin"},
			})
			// Then fail → the business tx must roll back.
			return errors.New("boom")
		},
	)
	if err == nil {
		t.Fatal("expected handler error to propagate")
	}

	// The business write was rolled back...
	got, gerr := fx.Users.FindByID(ctx, u.ID)
	if gerr != nil {
		t.Fatalf("find user: %v", gerr)
	}
	if got.Nickname != "victim" {
		t.Fatalf("business write must roll back: nickname=%q want victim", got.Nickname)
	}

	// ...but the audit row survived it.
	var auditCount int
	if e := fx.DB.DB().GetContext(ctx, &auditCount,
		`SELECT COUNT(*) FROM audit_log WHERE action='user.role_changed' AND target_id=?`, u.ID); e != nil {
		t.Fatalf("count audit rows: %v", e)
	}
	if auditCount != 1 {
		t.Fatalf("audit row must survive the business rollback, got %d", auditCount)
	}
}

// job-queue.md / shared.JobDispatcher promise: a job enqueued from a command
// handler joins the SAME transaction as the business write — they commit or
// roll back atomically. Proven through the real provider chain (JobDispatcher
// middleware injects a dispatcher whose Enqueue uses Conn(ctx)). Closes the
// untested infra-sched-job-16.
func TestCommandBus_JobEnqueueJoinsBusinessTransaction(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "job_tx.db"))

	registry, err := jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{
		"test.kind": func(context.Context, []byte) error { return nil },
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	dispatcher := jobapp.NewDispatcher(fx.Jobs, registry)
	cmdBus := newProductionCommandBus(t, fx, dispatcher)

	jobCount := func() int {
		var n int
		if e := fx.DB.DB().GetContext(ctx, &n, `SELECT COUNT(*) FROM jobs`); e != nil {
			t.Fatalf("count jobs: %v", e)
		}
		return n
	}

	// Rollback case: enqueue a job then fail → the job row must NOT persist,
	// proving the enqueue joined the rolled-back business tx.
	_ = bus.ExecVoid(
		ctx,
		cmdBus.Bus,
		"EnqueueThenFail",
		skipPermCmd{},
		func(ctx context.Context) error {
			if e := shared.JobDispatcherFromContext(ctx).Enqueue(ctx, "test.kind", 0, map[string]any{"x": 1}); e != nil {
				return e
			}
			return errors.New("boom")
		},
	)
	if n := jobCount(); n != 0 {
		t.Fatalf("job enqueue must roll back with the business tx, got %d job rows", n)
	}

	// Commit case: enqueue a job then succeed → the job row persists.
	if e := bus.ExecVoid(ctx, cmdBus.Bus, "EnqueueThenCommit", skipPermCmd{}, func(ctx context.Context) error {
		return shared.JobDispatcherFromContext(ctx).Enqueue(ctx, "test.kind", 0, map[string]any{"x": 2})
	}); e != nil {
		t.Fatalf("enqueue+commit: %v", e)
	}
	if n := jobCount(); n != 1 {
		t.Fatalf("job enqueue must commit with the business tx, got %d job rows", n)
	}
}
