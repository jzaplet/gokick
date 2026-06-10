// Package middleware_test holds the EXTERNAL test package for the bus
// middleware. It lives in package middleware_test (not middleware) on purpose:
// these tests reuse the shared gokick/app/internal/testfx fixture, and testfx
// itself imports gokick/app/application/bus/middleware — importing testfx from
// an in-package (white-box) test file would create an import cycle. An external
// test package breaks the cycle while still exercising the exported middleware
// against a real SQLite database. Helpers that the in-package tests keep
// unexported (silent logger, audit stubs, marker commands) are redeclared here
// with test-local names.
package middleware_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gokick/app/application/bus"
	mw "gokick/app/application/bus/middleware"
	"gokick/app/domain/job"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	"gokick/app/internal/testfx"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// auditDomainEvent is a minimal DomainEvent for the post-commit dispatch test.
type auditDomainEvent struct{ at time.Time }

func (auditDomainEvent) EventName() string       { return "test.event" }
func (e auditDomainEvent) OccurredAt() time.Time { return e.at }

// plainCmd implements NEITHER Permissioned nor SkipPermission — fine here
// because none of these chains include AuthorizeMiddleware.
type plainCmd struct{}

// ---------------------------------------------------------------------------
// app-events-audit-16 — an audit write persists even when the surrounding
// business transaction rolls back.
//
// The existing TestAuditMiddleware_FlushesEvenOnHandlerError proves the
// middleware DRAINS on error, but it uses a stub AuditLogger and no real DB,
// so it cannot prove the audit row actually lands in SQLite while the business
// write is rolled back. This wires the REAL production ordering —
// AuditMiddleware(outer) -> TransactionMiddleware(inner) -> handler — against a
// real SqliteManager, and asserts the business row is gone but the audit_log
// row is physically present.
// ---------------------------------------------------------------------------
func TestAuditMiddleware_PersistsAcrossBusinessRollback(t *testing.T) {
	// Not t.Parallel: tests calling testfx.New run goose migrations, and
	// goose sets process-global state (SetLogger/SetBaseFS) in RunUp — two
	// concurrent New() calls race on those globals under -race. Serializing
	// the DB-backed tests avoids it without touching production code.
	fx := testfx.New(t, t.TempDir()+"/audit_rollback.db")
	auditRepo := sqliteaudit.NewRepository(fx.DB) // real raw-pool AuditLogger

	audit := mw.AuditMiddleware(quietLogger(), auditRepo)
	txmw := mw.TransactionMiddleware(fx.DB)

	const action = "auth.login.failed"
	const nickname = "rollbackvictim"

	// handler writes a user row (joins the tx via r.Conn), records an audit
	// event, then fails -> the user row must roll back, the audit row must not.
	_, err := audit(
		context.Background(),
		"Login",
		plainCmd{},
		func(ctx context.Context) (any, error) {
			return txmw(ctx, "Login", plainCmd{}, func(ctx context.Context) (any, error) {
				seedUserInTx(t, fx, ctx, nickname)
				shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: action})
				return nil, errors.New("invalid credentials")
			})
		},
	)
	if err == nil || err.Error() != "invalid credentials" {
		t.Fatalf("handler error must propagate, got %v", err)
	}

	// Business row rolled back.
	if got := countRows(t, fx, `SELECT COUNT(*) FROM users WHERE nickname = ?`, nickname); got != 0 {
		t.Fatalf("business row must have rolled back, found %d users named %q", got, nickname)
	}

	// Audit row survived the rollback (raw pool, outside the tx).
	if got := countRows(t, fx, `SELECT COUNT(*) FROM audit_log WHERE action = ?`, action); got != 1 {
		t.Fatalf(
			"audit row must survive business rollback, found %d rows for action %q",
			got,
			action,
		)
	}
}

// ---------------------------------------------------------------------------
// app-events-audit-17 — each command gets its own audit collector; events do
// not leak between parallel commands sharing one AuditMiddleware instance.
//
// The domain-level TestAuditCollector_ConcurrentRecord only covers the
// collector struct; nothing pins the MIDDLEWARE's per-invocation isolation.
// Each concurrent invocation gets a UNIQUE actor (claims.UserID == its tag) and
// records an event whose TargetID is the SAME tag. The middleware stamps
// ActorUserID from the per-invocation context at flush time, while TargetID
// comes from whatever event the collector drained. If the collector were shared
// across invocations, an invocation acting as "i" could drain invocation "j"'s
// event, producing a record where ActorUserID != TargetID. We assert every
// persisted record has ActorUserID == TargetID (no cross-talk) and that all N
// tags are each persisted exactly once (none lost or duplicated).
// ---------------------------------------------------------------------------
func TestAuditMiddleware_PerRequestCollectorIsolation(t *testing.T) {
	t.Parallel()

	collated := &collatingAudit{count: make(map[string]int)}
	audit := mw.AuditMiddleware(quietLogger(), collated)

	const commands = 200
	var wg sync.WaitGroup
	wg.Add(commands)
	for i := 0; i < commands; i++ {
		i := i
		go func() {
			defer wg.Done()
			tag := strconv.Itoa(i)
			ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{UserID: tag})
			_, _ = audit(ctx, "Cmd", plainCmd{}, func(ctx context.Context) (any, error) {
				time.Sleep(time.Duration(i%5) * time.Microsecond) // maximise interleaving
				shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
					Action:   "test.tagged",
					TargetID: tag, // must travel with this invocation's actor
				})
				return nil, nil
			})
		}()
	}
	wg.Wait()

	collated.mu.Lock()
	defer collated.mu.Unlock()
	if collated.mismatches != 0 {
		t.Fatalf("%d record(s) had ActorUserID != TargetID — a leaked collector "+
			"flushed another command's event", collated.mismatches)
	}
	if got := collated.total; got != commands {
		t.Fatalf(
			"persisted records: got %d want %d (collector leaked or dropped events)",
			got,
			commands,
		)
	}
	for i := 0; i < commands; i++ {
		if c := collated.count[strconv.Itoa(i)]; c != 1 {
			t.Fatalf("tag %d persisted %d times (want exactly 1) — collector not per-request", i, c)
		}
	}
}

// ---------------------------------------------------------------------------
// app-events-audit-24 — when AuditLogger.Save fails, the failure is logged
// with action, command, and error fields.
//
// The existing PersistFailureDoesNotShadowHandlerResult uses a silent logger
// and only checks the handler result is preserved. This captures the slog
// output and asserts the three structured fields are present, pinning the
// failure-safe contract's diagnostic shape.
// ---------------------------------------------------------------------------
func TestAuditMiddleware_LogsSaveFailureWithFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	audit := mw.AuditMiddleware(logger, &failingAudit{})

	handler := func(ctx context.Context) (any, error) {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: "user.created"})
		return "ok", nil
	}

	got, err := audit(context.Background(), "CreateUser", plainCmd{}, handler)
	if err != nil {
		t.Fatalf("audit failure must not propagate, got %v", err)
	}
	if got != "ok" {
		t.Fatalf("handler result lost: %v", got)
	}

	logged := buf.String()
	if !strings.Contains(logged, "audit: write failed") {
		t.Fatalf("expected the write failure to be logged, got %q", logged)
	}
	if !strings.Contains(logged, `action=user.created`) {
		t.Fatalf("log must carry the event action, got %q", logged)
	}
	if !strings.Contains(logged, `command=CreateUser`) {
		t.Fatalf("log must carry the command name, got %q", logged)
	}
	if !strings.Contains(logged, "audit boom") {
		t.Fatalf("log must carry the underlying error, got %q", logged)
	}
}

// ---------------------------------------------------------------------------
// app-events-audit-30 — the flush runs on context.WithoutCancel, so a
// cancelled request context cannot abort the audit write.
//
// Save records the ctx.Err() it observes. We pass an ALREADY-cancelled context
// into the middleware; if the flush reused that context, Save would see
// context.Canceled. Asserting Save ran with a nil ctx.Err() proves the detach.
// ---------------------------------------------------------------------------
func TestAuditMiddleware_FlushUsesDetachedContext(t *testing.T) {
	t.Parallel()

	sentinel := &ctxErrAudit{}
	audit := mw.AuditMiddleware(quietLogger(), sentinel)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE the middleware runs

	handler := func(ctx context.Context) (any, error) {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: "auth.login.failed"})
		return nil, errors.New("creds")
	}

	_, _ = audit(ctx, "Login", plainCmd{}, handler)

	if !sentinel.saved {
		t.Fatal("Save must run even though the request context was cancelled")
	}
	if sentinel.ctxErr != nil {
		t.Fatalf(
			"flush must use a detached (non-cancelled) context, Save saw ctx.Err()=%v",
			sentinel.ctxErr,
		)
	}
}

// ---------------------------------------------------------------------------
// overview-54 — event handlers run as side-effects only AFTER a successful
// commit; a rollback drops them. Proven end-to-end against a real DB + real
// EventBus: the success path makes the committed row visible on the main pool
// AND fires the handler; the rollback path fires nothing and leaves no row.
// ---------------------------------------------------------------------------
func TestDispatchEventsMiddleware_AfterCommitSideEffectWithRealDB(t *testing.T) {
	// Not t.Parallel — see note in TestAuditMiddleware_PersistsAcrossBusinessRollback
	// (goose RunUp touches process-global state; serialize DB-backed tests).
	fx := testfx.New(t, t.TempDir()+"/events_commit.db")
	logger := quietLogger()

	run := func(t *testing.T, nickname string, handlerErr error) (fired bool, rowVisible bool) {
		eventBus := bus.NewEventBus(
			mw.RecoveryMiddleware(logger, shared.NopReporter{}),
			mw.LoggingMiddleware(logger),
		)
		var dispatched bool
		eventBus.Register("test.event", func(_ context.Context, _ shared.DomainEvent) error {
			dispatched = true
			return nil
		})

		dispatch := mw.DispatchEventsMiddleware(logger, eventBus)
		txmw := mw.TransactionMiddleware(fx.DB)

		_, _ = dispatch(
			context.Background(),
			"Cmd",
			plainCmd{},
			func(ctx context.Context) (any, error) {
				return txmw(ctx, "Cmd", plainCmd{}, func(ctx context.Context) (any, error) {
					seedUserInTx(t, fx, ctx, nickname) // write joins the tx via r.Conn
					shared.EventCollectorFromContext(ctx).Collect(auditDomainEvent{at: time.Now()})
					return nil, handlerErr
				})
			},
		)

		visible := countRows(t, fx, `SELECT COUNT(*) FROM users WHERE nickname = ?`, nickname) == 1
		return dispatched, visible
	}

	t.Run("successful commit dispatches and persists", func(t *testing.T) {
		fired, visible := run(t, "committeduser", nil)
		if !fired {
			t.Fatal("event handler must fire after a successful commit")
		}
		if !visible {
			t.Fatal(
				"committed row must be visible on the main pool (dispatch happened post-commit)",
			)
		}
	})

	t.Run("handler error rolls back and dispatches nothing", func(t *testing.T) {
		fired, visible := run(t, "rolledbackuser", errors.New("boom"))
		if fired {
			t.Fatal("event handler must NOT fire when the tx rolls back")
		}
		if visible {
			t.Fatal("rolled-back row must not be visible on the main pool")
		}
	})
}

// ---------------------------------------------------------------------------
// roadmap-39 — JobDispatcherMiddleware injects the dispatcher into ctx BEFORE
// TransactionMiddleware, so an Enqueue inside a handler joins the business tx:
// a handler error rolls the job back; a clean commit persists it.
//
// testRef was "none". Wires the real chain order
// JobDispatcherMiddleware(outer) -> TransactionMiddleware(real DB)(inner) and a
// dispatcher that writes through the real job.Repository (Conn-aware), then
// asserts the jobs table reflects the tx outcome.
// ---------------------------------------------------------------------------
func TestJobDispatcherMiddleware_EnqueueJoinsBusinessTx(t *testing.T) {
	// Not t.Parallel — see note in TestAuditMiddleware_PersistsAcrossBusinessRollback
	// (goose RunUp touches process-global state; serialize DB-backed tests).
	fx := testfx.New(t, t.TempDir()+"/job_tx.db")

	dispatcher := &txJoiningDispatcher{repo: fx.Jobs}
	jobmw := mw.JobDispatcherMiddleware(dispatcher)
	txmw := mw.TransactionMiddleware(fx.DB)

	runOnce := func(kind string, handlerErr error) {
		_, _ = jobmw(
			context.Background(),
			"Cmd",
			plainCmd{},
			func(ctx context.Context) (any, error) {
				return txmw(ctx, "Cmd", plainCmd{}, func(ctx context.Context) (any, error) {
					if err := shared.JobDispatcherFromContext(ctx).Enqueue(ctx, kind, 0, nil); err != nil {
						return nil, err
					}
					return nil, handlerErr
				})
			},
		)
	}

	// Rollback path: handler errors -> the enqueued job must roll back with it.
	runOnce("rolledbackjob", errors.New("boom"))
	if n := countRows(t, fx, `SELECT COUNT(*) FROM jobs WHERE kind = ?`, "rolledbackjob"); n != 0 {
		t.Fatalf("Enqueue must join the business tx and roll back, found %d job rows", n)
	}

	// Success path: clean commit -> the enqueued job persists.
	runOnce("committedjob", nil)
	if n := countRows(t, fx, `SELECT COUNT(*) FROM jobs WHERE kind = ?`, "committedjob"); n != 1 {
		t.Fatalf("Enqueue on a clean commit must persist, found %d job rows", n)
	}
}

// --- test doubles & helpers ------------------------------------------------

// seedUserInTx writes a user row through the real repository using the given
// (transaction-carrying) context, so the INSERT joins the surrounding tx and
// rolls back with it.
func seedUserInTx(t *testing.T, fx *testfx.Fixture, ctx context.Context, nickname string) {
	t.Helper()
	hash, err := fx.Hasher.Hash("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	nn, err := user.NewNickname(nickname)
	if err != nil {
		t.Fatalf("nickname: %v", err)
	}
	r, err := user.NewRole("user")
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	em, err := user.NewEmail(nickname + "@example.com")
	if err != nil {
		t.Fatalf("email: %v", err)
	}
	if err := fx.Users.Save(ctx, user.NewUser(nn, hash, em, r)); err != nil {
		t.Fatalf("save user in tx: %v", err)
	}
}

func countRows(t *testing.T, fx *testfx.Fixture, query string, args ...any) int {
	t.Helper()
	var n int
	if err := fx.DB.DB().GetContext(context.Background(), &n, query, args...); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

// collatingAudit records per-TargetID counts plus a mismatch tally (records
// whose stamped ActorUserID disagrees with their TargetID) so the isolation
// test can prove no event leaked across per-request collectors.
type collatingAudit struct {
	mu         sync.Mutex
	total      int
	mismatches int
	count      map[string]int
}

func (c *collatingAudit) Save(_ context.Context, rec *shared.AuditRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	tag := ""
	if rec.TargetID != nil {
		tag = *rec.TargetID
	}
	actor := ""
	if rec.ActorUserID != nil {
		actor = *rec.ActorUserID
	}
	if actor != tag {
		c.mismatches++
	}
	c.total++
	c.count[tag]++
	return nil
}

// failingAudit always fails Save, mimicking a degraded audit backend.
type failingAudit struct{}

func (*failingAudit) Save(context.Context, *shared.AuditRecord) error {
	return errors.New("audit boom")
}

// ctxErrAudit captures whether Save ran and what ctx.Err() it observed.
type ctxErrAudit struct {
	saved  bool
	ctxErr error
}

func (c *ctxErrAudit) Save(ctx context.Context, _ *shared.AuditRecord) error {
	c.saved = true
	c.ctxErr = ctx.Err()
	return nil
}

// txJoiningDispatcher is a minimal shared.JobDispatcher that persists through
// the real job.Repository. Its Enqueue uses r.Conn(ctx), so when invoked inside
// a transaction the INSERT joins that transaction — exactly what the middleware
// chain ordering is supposed to make possible.
type txJoiningDispatcher struct {
	repo job.Repository
}

func (d *txJoiningDispatcher) Enqueue(
	ctx context.Context,
	kind string,
	maxRetries int,
	_ any,
	_ ...shared.EnqueueOption,
) error {
	return d.repo.Enqueue(ctx, job.NewJob(kind, []byte("null"), maxRetries))
}
