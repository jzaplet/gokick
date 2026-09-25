package middleware

import (
	"context"
	"errors"
	"testing"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// runPlane runs mw around a handler and returns the plane the handler saw.
func runPlane(t *testing.T, ctx context.Context, mw bus.Middleware, cmd any) shared.Plane {
	t.Helper()
	var got shared.Plane
	if _, err := mw(ctx, "Cmd", cmd, func(ctx context.Context) (any, error) {
		got = shared.PlaneFromContext(ctx)
		return nil, nil
	}); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	return got
}

// The plane follows the declared permission: platform:* is the platform plane,
// every other permission — and SkipPermission — the tenant plane.
func TestPlaneMiddleware_DerivesThePlaneFromThePermission(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		cmd  any
		want shared.Plane
	}{
		{"platform permission", permitCmd{perm: "platform:overview"}, shared.PlanePlatform},
		{"admin permission", permitCmd{perm: "admin:users:read"}, shared.PlaneTenant},
		{"user permission", permitCmd{perm: "profile:read"}, shared.PlaneTenant},
		{"skip permission", noopCommand{}, shared.PlaneTenant},
	} {
		if got := runPlane(t, t.Context(), PlaneMiddleware(), tc.cmd); got != tc.want {
			t.Errorf("%s: plane = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// The plane is set, never inherited: a tenant command dispatched from inside
// cross-tenant work still runs on the tenant plane.
func TestPlaneMiddleware_OverridesAnInheritedPlane(t *testing.T) {
	t.Parallel()
	ctx := shared.ContextWithPlane(t.Context(), shared.PlaneSystem)
	if got := runPlane(t, ctx, PlaneMiddleware(), permitCmd{perm: "admin:users:read"}); got != shared.PlaneTenant {
		t.Fatalf("plane = %s, want tenant", got)
	}
}

func TestSystemPlaneMiddleware_MarksTheSystemPlane(t *testing.T) {
	t.Parallel()
	if got := runPlane(t, t.Context(), SystemPlaneMiddleware(), noopCommand{}); got != shared.PlaneSystem {
		t.Fatalf("plane = %s, want system", got)
	}
}

// The chains carry the plane to the handler — and so to the transaction the
// chain opens: the production chains, not the middleware in isolation.
func TestChains_CarryThePlaneToTheHandler(t *testing.T) {
	t.Parallel()
	logger := silent()
	resolver := stubTenantResolver{id: shared.DefaultTenantID}
	superadmin := shared.ContextWithClaims(t.Context(),
		&shared.AuthClaims{UserID: "u1", Role: shared.RoleSuperAdmin})

	seen := func(ctx context.Context, b *bus.QueryBus, cmd any) shared.Plane {
		t.Helper()
		got, err := bus.Query(ctx, b, "Q", cmd, func(ctx context.Context) (shared.Plane, error) {
			return shared.PlaneFromContext(ctx), nil
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		return got
	}
	queryBus := bus.NewQueryBus(
		QueryChain(logger, &stubChecker{}, shared.NopReporter{}, resolver, &stubTx{})...)
	if got := seen(superadmin, queryBus, permitCmd{perm: "platform:overview"}); got != shared.PlanePlatform {
		t.Errorf("QueryChain, platform query: plane = %s, want platform", got)
	}
	if got := seen(superadmin, queryBus, permitCmd{perm: "admin:users:read"}); got != shared.PlaneTenant {
		t.Errorf("QueryChain, admin query: plane = %s, want tenant", got)
	}

	eventBus := bus.NewEventBus()
	systemBus := bus.NewSystemCommandBus(SystemChain(logger, &stubTx{}, eventBus,
		&captureAudit{}, shared.RunDispatcherFromContext(t.Context()), shared.NopReporter{})...)
	got, err := bus.SystemDispatch(
		t.Context(),
		systemBus,
		"C",
		noopCommand{},
		func(ctx context.Context) (shared.Plane, error) { return shared.PlaneFromContext(ctx), nil },
	)
	if err != nil {
		t.Fatalf("system dispatch: %v", err)
	}
	if got != shared.PlaneSystem {
		t.Errorf("SystemChain: plane = %s, want system", got)
	}
}

// ReadTx wraps the query in exactly one read transaction and always ends it —
// on success and on a handler error alike.
func TestReadTxMiddleware_OpensAndEndsOneReadTx(t *testing.T) {
	t.Parallel()
	for _, handlerErr := range []error{nil, errors.New("boom")} {
		tx := &stubTx{}
		_, err := ReadTxMiddleware(tx)(t.Context(), "Q", noopCommand{},
			func(context.Context) (any, error) { return nil, handlerErr })
		if !errors.Is(err, handlerErr) {
			t.Fatalf("handler error must pass through unchanged: got %v, want %v", err, handlerErr)
		}
		if tx.readBeginCalls != 1 || tx.readEndCalls != 1 {
			t.Fatalf("handler err %v: began %d, ended %d read tx, want 1/1",
				handlerErr, tx.readBeginCalls, tx.readEndCalls)
		}
		if tx.beginCalls != 0 || tx.commitCalls != 0 || tx.rollbackCalls != 0 {
			t.Fatal("a query must not open a write transaction")
		}
	}
}

// A read transaction that cannot open fails the query before the handler runs.
func TestReadTxMiddleware_BeginErrorStopsTheQuery(t *testing.T) {
	t.Parallel()
	beginErr := errors.New("no connection")
	var ran bool
	_, err := ReadTxMiddleware(&stubTx{beginErr: beginErr})(t.Context(), "Q", noopCommand{},
		func(context.Context) (any, error) { ran = true; return nil, nil })
	if !errors.Is(err, beginErr) {
		t.Fatalf("err = %v, want the BeginReadTx error", err)
	}
	if ran {
		t.Fatal("the handler must not run without its read transaction")
	}
}
