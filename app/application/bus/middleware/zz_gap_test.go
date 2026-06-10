package middleware

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// app-bus-33 — Queries pass through QueryBus with chain
// Recovery -> Logging -> Authorize.
//
// Every other middleware test exercises AuthorizeMiddleware in isolation or via
// a CommandBus. Nothing pins that the *QueryBus* actually carries Authorize.
// This builds a QueryBus from the real BaseChain helper (the same triplet
// container_provider hands the production QueryBus) and dispatches a
// Permissioned query whose checker DENIES. The denial can only surface if
// AuthorizeMiddleware is present in the QueryBus chain: drop Authorize from
// BaseChain and the handler would run and return success instead.
//
// We also assert the handler did NOT run (fail-closed) and the checker WAS
// consulted with the query's required permission — proving the request reached
// Authorize rather than short-circuiting elsewhere.
// ---------------------------------------------------------------------------
func TestQueryBus_AuthorizeDeniesUnpermittedQuery(t *testing.T) {
	t.Parallel()

	denied := &shared.PermissionError{Message: "insufficient permissions"}
	checker := &stubChecker{err: denied}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	queryBus := bus.NewQueryBus(BaseChain(logger, checker, shared.NopReporter{})...)

	var ran bool
	_, err := bus.Exec(
		t.Context(),
		queryBus.Bus,
		"ListUsers",
		permitCmd{perm: "admin:users:read"},
		func(context.Context) (string, error) {
			ran = true
			return "rows", nil
		},
	)

	var permErr *shared.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf(
			"QueryBus must run Authorize and surface the denial as *PermissionError, got %v",
			err,
		)
	}
	if ran {
		t.Fatal("handler must NOT run when the query is denied (QueryBus is fail-closed)")
	}
	if !checker.called || checker.gotPerm != "admin:users:read" {
		t.Fatalf("Authorize must consult the checker with the query permission; called=%v perm=%q",
			checker.called, checker.gotPerm)
	}
}

// A QueryBus query that declares NEITHER Permissioned nor SkipPermission must
// be rejected by the QueryBus's Authorize stage (same fail-closed default as
// commands — the LEDGER lists app-bus-35 / domain-28 / guide-auth-perm-51 for
// the *query* side specifically). Proves the guard is reached through the real
// QueryBus chain, not only when AuthorizeMiddleware is invoked bare.
func TestQueryBus_RejectsQueryWithoutDeclaration(t *testing.T) {
	t.Parallel()

	checker := &stubChecker{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	queryBus := bus.NewQueryBus(BaseChain(logger, checker, shared.NopReporter{})...)

	var ran bool
	_, err := bus.Exec(
		t.Context(),
		queryBus.Bus,
		"BareQuery",
		bareCmd{}, // implements neither marker
		func(context.Context) (string, error) {
			ran = true
			return "rows", nil
		},
	)
	if err == nil {
		t.Fatal("a query implementing neither Permissioned nor SkipPermission must be rejected")
	}
	if ran {
		t.Fatal("handler must NOT run for an undeclared query (fail-closed)")
	}
	if checker.called {
		t.Fatal("checker must not be consulted for an undeclared query")
	}
}

// ---------------------------------------------------------------------------
// app-events-audit-07 — when an event handler fails, the command has ALREADY
// committed; the error is logged and the caller is unaffected.
//
// EventBus.Dispatch swallows handler errors (`_ = ExecVoid(...)`, event.go:41),
// and DispatchEventsMiddleware flushes only AFTER next() returns success. So a
// failing post-commit side-effect must NOT turn the command into an error.
// Existing event tests all register handlers that return nil — none pin the
// failure path. This wraps DispatchEvents over a handler that Collects an event
// then succeeds, registers an EventBus handler that ERRORS, and asserts:
//   - the command result/err are untouched (success), and
//   - the failing handler actually ran (the error path was exercised).
//
// Mutation guard: if DispatchEventsMiddleware propagated eventBus errors
// (e.g. returned a dispatch error instead of the handler's nil), this fails.
// ---------------------------------------------------------------------------
func TestDispatchEventsMiddleware_EventHandlerErrorDoesNotFailCommand(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventBus := bus.NewEventBus(
		RecoveryMiddleware(logger, shared.NopReporter{}),
		LoggingMiddleware(logger),
	)

	var handlerRan bool
	eventBus.Register("test.event", func(_ context.Context, _ shared.DomainEvent) error {
		handlerRan = true
		return errors.New("side-effect boom")
	})

	dispatch := DispatchEventsMiddleware(logger, eventBus)

	result, err := dispatch(
		t.Context(),
		"Cmd",
		normalCmd{},
		func(ctx context.Context) (any, error) {
			shared.EventCollectorFromContext(ctx).Collect(testEvent{dispatchID: 1, at: time.Now()})
			return "command-ok", nil
		},
	)
	if err != nil {
		t.Fatalf(
			"a failing event handler must not fail the (already-committed) command, got %v",
			err,
		)
	}
	if result != "command-ok" {
		t.Fatalf("command result must be preserved, got %v", result)
	}
	if !handlerRan {
		t.Fatal("the erroring event handler must have actually run (failure path not exercised)")
	}
}

// ---------------------------------------------------------------------------
// app-events-audit-31 — the AuditMiddleware fills id (UUID) and created_at on
// the AuditRecord; the handler supplies only the AuditEvent fields (Action, …).
//
// TestAuditMiddleware_StampsActorAndIPFromContext already pins actor_user_id,
// actor_ip, target_id and metadata, but NOT the two fields the middleware
// generates itself: a fresh UUID id and the flush-time created_at. The handler
// records an event carrying ONLY an Action; we assert the persisted record has
// a parseable non-nil UUID id and a non-zero created_at — neither of which the
// handler supplied. Mutation guard: dropping `ID: uuid.New().String()` or
// `CreatedAt: now` in writeRecord (audit.go) leaves an empty PK / zero time and
// fails this test.
// ---------------------------------------------------------------------------
func TestAuditMiddleware_FillsRecordIDAndCreatedAt(t *testing.T) {
	t.Parallel()

	audit := &captureAudit{}
	mw := AuditMiddleware(silent(), audit)

	before := time.Now()
	handler := func(ctx context.Context) (any, error) {
		// Handler supplies ONLY the action; id/created_at are the middleware's job.
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: "user.created"})
		return nil, nil
	}
	if _, err := mw(t.Context(), "CreateUser", nil, handler); err != nil {
		t.Fatalf("handler: %v", err)
	}
	after := time.Now()

	recs := audit.drained()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 record, got %d", len(recs))
	}
	rec := recs[0]

	if rec.ID == "" {
		t.Fatal("middleware must fill a generated UUID id; got empty")
	}
	if _, err := uuid.Parse(rec.ID); err != nil {
		t.Fatalf("id must be a valid UUID, got %q (%v)", rec.ID, err)
	}
	if rec.CreatedAt.IsZero() {
		t.Fatal("middleware must stamp created_at; got zero time")
	}
	// created_at is stamped at flush time, which sits between before/after.
	if rec.CreatedAt.Before(before) || rec.CreatedAt.After(after) {
		t.Fatalf("created_at %v must fall within [%v, %v]", rec.CreatedAt, before, after)
	}
}
