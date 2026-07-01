package di

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/application/bus"
	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
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
// which omits Audit) silently can.
func newProductionCommandBus(
	t *testing.T,
	fx *testfx.Fixture,
	runDispatcher shared.RunDispatcher,
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
		runDispatcher,
		audit,
		shared.NopReporter{},
		security.NewDefaultTenantResolver(),
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

// The run dispatcher's promise: a durable
// run enqueued from a command handler joins that handler's transaction — the
// INSERT into runs commits or rolls back atomically with the business write.
// Proven through the real provider chain (RunDispatcherMiddleware injects a
// dispatcher whose repo.Enqueue uses Conn(ctx)), so a missing or misordered
// RunDispatcherMiddleware relative to TransactionMiddleware fails here. With a
// noop run dispatcher (as the other tests use) the guarantee would be untested.
func TestCommandBus_RunEnqueueJoinsBusinessTransaction(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "run_tx.db"))

	registry, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{
		"test.run": {
			Handler: func(context.Context, *run.Run, runapp.Checkpointer) error { return nil },
		},
	}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	dispatcher := runapp.NewDispatcher(fx.Runs, registry)
	cmdBus := newProductionCommandBus(t, fx, dispatcher)

	runCount := func() int {
		var n int
		if e := fx.DB.DB().GetContext(ctx, &n, `SELECT COUNT(*) FROM runs`); e != nil {
			t.Fatalf("count runs: %v", e)
		}
		return n
	}

	// Rollback case: enqueue a run then fail → the run row must NOT persist,
	// proving the enqueue joined the rolled-back business tx.
	_ = bus.ExecVoid(
		ctx,
		cmdBus.Bus,
		"EnqueueRunThenFail",
		skipPermCmd{},
		func(ctx context.Context) error {
			if e := shared.RunDispatcherFromContext(ctx).Enqueue(ctx, "test.run", 0, map[string]any{"x": 1}); e != nil {
				return e
			}
			return errors.New("boom")
		},
	)
	if n := runCount(); n != 0 {
		t.Fatalf("run enqueue must roll back with the business tx, got %d run rows", n)
	}

	// Commit case: enqueue a run then succeed → the run row persists.
	if e := bus.ExecVoid(ctx, cmdBus.Bus, "EnqueueRunThenCommit", skipPermCmd{}, func(ctx context.Context) error {
		return shared.RunDispatcherFromContext(ctx).Enqueue(ctx, "test.run", 0, map[string]any{"x": 2})
	}); e != nil {
		t.Fatalf("enqueue+commit: %v", e)
	}
	if n := runCount(); n != 1 {
		t.Fatalf("run enqueue must commit with the business tx, got %d run rows", n)
	}
}

// Tenant spine: TenantMiddleware (in BaseChain) must thread the resolved tenant
// into the handler's ctx through the REAL production command chain — proving the
// spine is wired, not just unit-correct. Single-tenant mode yields the default
// tenant. Closes the gap a hand-assembled chain would miss.
func TestCommandBus_InjectsTenantIntoHandlerContext(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_ctx.db"))
	cmdBus := newProductionCommandBus(t, fx, noopDispatcher{})

	var seen string
	err := bus.ExecVoid(
		ctx,
		cmdBus.Bus,
		"ReadTenant",
		skipPermCmd{},
		func(ctx context.Context) error {
			seen = shared.TenantIDFromContext(ctx)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != shared.DefaultTenantID {
		t.Fatalf("handler ctx tenant = %q, want default %q", seen, shared.DefaultTenantID)
	}
}
