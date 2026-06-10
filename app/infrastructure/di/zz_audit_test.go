package di

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/scheduler"
	"gokick/app/infrastructure/security"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	"gokick/app/internal/testfx"
	httpmw "gokick/app/presentation/http/middleware"
)

// deniedAdminCmd is a Permissioned command requiring an admin permission. With
// no claims in ctx the production PermissionChecker rejects it — used to prove
// AuthorizeMiddleware is wired into the production CommandBus/QueryBus chains.
type deniedAdminCmd struct{}

func (deniedAdminCmd) RequiredPermission() string { return "admin:users:create" }

// rowVisibleEvent is a minimal domain event carrying only a primitive nickname,
// dispatched after a command commits so the event handler can assert the
// business row is already visible on a separate pool connection.
type rowVisibleEvent struct{ nickname string }

func (rowVisibleEvent) EventName() string     { return "di.test.row_visible" }
func (rowVisibleEvent) OccurredAt() time.Time { return time.Now() }

// --- roadmap-05 / app-bus-10 / overview-14: AuthorizeMiddleware in the
// production CommandBus chain blocks a denied command before its handler runs.
//
// Dispatched through the real provideCommandBus chain (not a hand-rolled one),
// so removing AuthorizeMiddleware from the production wiring flips this test.
func TestCommandBus_AuthorizeBlocksDeniedCommand(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "authz_cmd.db"))
	cmdBus := newProductionCommandBus(t, fx, noopDispatcher{})

	// Authenticated but non-admin caller: the checker reaches the role gate and
	// denies the admin-only command with a PermissionError (not AuthError).
	ctx := shared.ContextWithClaims(
		context.Background(),
		&shared.AuthClaims{UserID: "u1", Role: "user"},
	)

	var handlerRan bool
	err := bus.ExecVoid(
		ctx,
		cmdBus.Bus,
		"DeniedCreate",
		deniedAdminCmd{},
		func(context.Context) error {
			handlerRan = true
			return nil
		},
	)

	var permErr *shared.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("denied command must yield *shared.PermissionError, got %v", err)
	}
	if handlerRan {
		t.Fatal("handler must not run when AuthorizeMiddleware denies the command")
	}
}

// --- roadmap-05 / app-bus-10 / overview-14 / app-bus-15:
// DispatchEventsMiddleware sits OUTSIDE TransactionMiddleware in the production
// chain, so collected events are flushed only AFTER the business commit.
//
// The discriminating assertion is on the SUCCESS path: inside the event handler
// we read the just-written user row on a SEPARATE pool connection
// (context.Background(), no tx). If DispatchEvents wraps Transaction (correct),
// the commit has already returned before dispatch and the row is visible. If
// the two were swapped so events fired inside the tx, a separate connection
// would see nothing (uncommitted) — flipping this test. A rollback-only check
// can't tell the two apart, so the visibility check is the real proof.
func TestCommandBus_EventsDispatchAfterCommit(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "events_after_commit.db"))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := security.NewPermissionChecker()
	eventBus := provideEventBus(logger, provideEventHandlers(), shared.NopReporter{})

	var (
		handlerFired atomic.Int32
		rowVisible   atomic.Bool
	)
	eventBus.Register("di.test.row_visible", func(_ context.Context, e shared.DomainEvent) error {
		handlerFired.Add(1)
		ev := e.(rowVisibleEvent)
		// Separate connection on a bare context — sees only committed data.
		existing, ferr := fx.Users.FindByNickname(context.Background(), ev.nickname)
		if ferr == nil && existing != nil {
			rowVisible.Store(true)
		}
		return nil
	})

	cmdBus := provideCommandBus(
		logger,
		fx.DB,
		checker,
		eventBus,
		noopDispatcher{},
		sqliteaudit.NewRepository(fx.DB),
		shared.NopReporter{},
	)

	const nick = "committed-user"
	err := bus.ExecVoid(
		ctx,
		cmdBus.Bus,
		"CreateThenEmit",
		skipPermCmd{},
		func(ctx context.Context) error {
			nn, e := user.NewNickname(nick)
			if e != nil {
				return e
			}
			em, e := user.NewEmail(nick + "@example.com")
			if e != nil {
				return e
			}
			r, e := user.NewRole("user")
			if e != nil {
				return e
			}
			u := user.NewUser(nn, "hash", em, r)
			if e := fx.Users.Save(ctx, u); e != nil { // joins the bus tx via Conn(ctx)
				return e
			}
			shared.EventCollectorFromContext(ctx).Collect(rowVisibleEvent{nickname: nick})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if handlerFired.Load() != 1 {
		t.Fatalf("event handler must fire exactly once after commit, fired %d", handlerFired.Load())
	}
	if !rowVisible.Load() {
		t.Fatal(
			"business row must be committed (visible on a separate connection) before events dispatch — DispatchEvents must wrap Transaction",
		)
	}
}

// --- roadmap-05 / app-bus-10 / overview-14:
// On a handler error the production chain rolls back the business write AND
// discards the collected event (it is never dispatched). Complements the
// success-path test above to pin the commit-gated event flow end to end.
func TestCommandBus_EventsDiscardedOnRollback(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "events_rollback.db"))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := security.NewPermissionChecker()
	eventBus := provideEventBus(logger, provideEventHandlers(), shared.NopReporter{})

	var handlerFired atomic.Int32
	eventBus.Register("di.test.row_visible", func(context.Context, shared.DomainEvent) error {
		handlerFired.Add(1)
		return nil
	})

	cmdBus := provideCommandBus(
		logger,
		fx.DB,
		checker,
		eventBus,
		noopDispatcher{},
		sqliteaudit.NewRepository(fx.DB),
		shared.NopReporter{},
	)

	const nick = "rolledback-user"
	err := bus.ExecVoid(
		ctx,
		cmdBus.Bus,
		"CreateThenFail",
		skipPermCmd{},
		func(ctx context.Context) error {
			nn, _ := user.NewNickname(nick)
			em, _ := user.NewEmail(nick + "@example.com")
			r, _ := user.NewRole("user")
			u := user.NewUser(nn, "hash", em, r)
			if e := fx.Users.Save(ctx, u); e != nil {
				return e
			}
			shared.EventCollectorFromContext(ctx).Collect(rowVisibleEvent{nickname: nick})
			return errors.New("boom")
		},
	)
	if err == nil {
		t.Fatal("expected handler error to propagate")
	}

	if handlerFired.Load() != 0 {
		t.Fatalf("event must be discarded on rollback, handler fired %d times", handlerFired.Load())
	}
	// The business write was rolled back too.
	got, ferr := fx.Users.FindByNickname(context.Background(), nick)
	if ferr != nil {
		t.Fatalf("find user: %v", ferr)
	}
	if got != nil {
		t.Fatal("business row must be rolled back on handler error")
	}
}

// --- app-bus-33: the production QueryBus runs AuthorizeMiddleware — a denied
// Permissioned query is rejected before its handler runs, and a permitted query
// reaches its handler.
func TestQueryBus_AuthorizeEnforced(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := security.NewPermissionChecker()
	queryBus := provideQueryBus(logger, checker, shared.NopReporter{})

	// Denied: authenticated non-admin caller → role gate rejects the admin query.
	userCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{UserID: "u1", Role: "user"})
	var deniedHandlerRan bool
	_, err := bus.Exec[int](
		userCtx,
		queryBus.Bus,
		"DeniedQuery",
		deniedAdminCmd{},
		func(context.Context) (int, error) {
			deniedHandlerRan = true
			return 1, nil
		},
	)
	var permErr *shared.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("denied query must yield *shared.PermissionError, got %v", err)
	}
	if deniedHandlerRan {
		t.Fatal("query handler must not run when AuthorizeMiddleware denies the query")
	}

	// Permitted: admin claims satisfy the same permission → handler runs.
	adminCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{UserID: "u1", Role: "admin"})
	got, err := bus.Exec[int](
		adminCtx,
		queryBus.Bus,
		"AllowedQuery",
		deniedAdminCmd{},
		func(context.Context) (int, error) {
			return 42, nil
		},
	)
	if err != nil {
		t.Fatalf("permitted query must pass Authorize: %v", err)
	}
	if got != 42 {
		t.Fatalf("permitted query handler result: got %d want 42", got)
	}
}

// --- roadmap-22: the single registered scheduler job is
// "cleanup:expired-refresh-tokens" with a 1h interval whose Fn delegates to
// token.DeleteExpired. provideSchedulerJobs is the production source of truth.
func TestProvideSchedulerJobs_RefreshTokenCleanup(t *testing.T) {
	spy := &deleteExpiredSpy{}
	jobs := provideSchedulerJobs(spy)

	if len(jobs) != 1 {
		t.Fatalf("expected exactly 1 scheduler job, got %d", len(jobs))
	}
	j := jobs[0]
	if j.Name != "cleanup:expired-refresh-tokens" {
		t.Fatalf("job name: got %q want cleanup:expired-refresh-tokens", j.Name)
	}
	if j.Interval != time.Hour {
		t.Fatalf("job interval: got %s want 1h", j.Interval)
	}
	if j.Fn == nil {
		t.Fatal("job Fn must not be nil")
	}
	if err := j.Fn(context.Background()); err != nil {
		t.Fatalf("job Fn returned error: %v", err)
	}
	if spy.calls.Load() != 1 {
		t.Fatalf("job Fn must delegate to token.DeleteExpired, calls=%d", spy.calls.Load())
	}
}

// --- roadmap-25: provideScheduler surfaces NewScheduler's validation error
// (here a duplicate job name) instead of swallowing it — this is the error that
// CreateApplication relies on bubbling up for fail-fast startup.
func TestProvideScheduler_PropagatesValidationError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	noop := func(context.Context) error { return nil }
	jobs := []scheduler.Job{
		{Name: "dup", Interval: time.Second, Fn: noop},
		{Name: "dup", Interval: time.Second, Fn: noop},
	}
	if _, err := provideScheduler(logger, jobs); err == nil {
		t.Fatal("provideScheduler must propagate the duplicate-name validation error")
	}
}

// --- roadmap-69: provideIPExtractor is the single source of client-IP
// resolution, and the SAME extractor governs both the audit IPMiddleware
// (ActorIP) and the rate limiter's bucket key. Flipping APP_TRUST_PROXY_HEADERS
// flips both together.
//
// trustProxy=true: X-Real-IP wins. A 1-token limiter built with this extractor
// returns 200 then 429 for two requests sharing X-Real-IP but differing in
// RemoteAddr (keyed on X-Real-IP), and IPMiddleware stamps ActorIP=X-Real-IP.
// trustProxy=false: RemoteAddr wins. Same RemoteAddr with differing X-Real-IP
// trips the second request, and ActorIP=RemoteAddr's host.
func TestProvideIPExtractor_SharedByRateLimitAndAudit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	oneToken := httpmw.RateRule{Tokens: 1, Per: time.Minute}

	// --- trustProxy = true: X-Real-IP is authoritative for BOTH consumers.
	extractTrust := provideIPExtractor(&config.Config{TrustProxyHeaders: true})

	gotActorTrust := actorIPFor(t, extractTrust, "9.9.9.9", "203.0.113.7:5555")
	if gotActorTrust != "9.9.9.9" {
		t.Fatalf("trustProxy=true: ActorIP got %q want 9.9.9.9 (X-Real-IP)", gotActorTrust)
	}
	limTrust := httpmw.NewRateLimiter(oneToken, extractTrust, logger)
	// Same X-Real-IP, different RemoteAddr → keyed on X-Real-IP → 2nd is 429.
	if code := rateCode(limTrust, "9.9.9.9", "1.1.1.1:1111"); code != http.StatusOK {
		t.Fatalf("trustProxy=true: first request got %d want 200", code)
	}
	if code := rateCode(limTrust, "9.9.9.9", "2.2.2.2:2222"); code != http.StatusTooManyRequests {
		t.Fatalf(
			"trustProxy=true: second request (same X-Real-IP) got %d want 429 — limiter must key on the extractor's IP",
			code,
		)
	}

	// --- trustProxy = false: RemoteAddr is authoritative; X-Real-IP ignored.
	extractNoTrust := provideIPExtractor(&config.Config{TrustProxyHeaders: false})

	gotActorNoTrust := actorIPFor(t, extractNoTrust, "9.9.9.9", "203.0.113.7:5555")
	if gotActorNoTrust != "203.0.113.7" {
		t.Fatalf(
			"trustProxy=false: ActorIP got %q want 203.0.113.7 (RemoteAddr host)",
			gotActorNoTrust,
		)
	}
	limNoTrust := httpmw.NewRateLimiter(oneToken, extractNoTrust, logger)
	// Same RemoteAddr, different X-Real-IP → keyed on RemoteAddr → 2nd is 429.
	if code := rateCode(limNoTrust, "7.7.7.7", "203.0.113.7:5555"); code != http.StatusOK {
		t.Fatalf("trustProxy=false: first request got %d want 200", code)
	}
	if code := rateCode(limNoTrust, "8.8.8.8", "203.0.113.7:5555"); code != http.StatusTooManyRequests {
		t.Fatalf(
			"trustProxy=false: second request (same RemoteAddr) got %d want 429 — limiter must key on RemoteAddr when proxy headers are untrusted",
			code,
		)
	}
}

// actorIPFor drives one request with the given X-Real-IP and RemoteAddr through
// IPMiddleware(extract) and returns the ActorIP the audit layer would stamp.
func actorIPFor(t *testing.T, extract httpmw.IPExtractor, realIP, remoteAddr string) string {
	t.Helper()
	var got string
	h := httpmw.IPMiddleware(
		extract,
	)(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = shared.ActorIPFromContext(r.Context())
		}),
	)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	if realIP != "" {
		req.Header.Set("X-Real-IP", realIP)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

// rateCode drives one request with the given X-Real-IP and RemoteAddr through
// the limiter's middleware and returns the resulting status code.
func rateCode(lim *httpmw.RateLimiter, realIP, remoteAddr string) int {
	h := lim.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	if realIP != "" {
		req.Header.Set("X-Real-IP", realIP)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// deleteExpiredSpy is a TokenRepository that records DeleteExpired invocations,
// proving the scheduler job's Fn is wired to token.DeleteExpired. (Named
// distinctly from scheduler_test.go's stubTokens to avoid a redeclaration.)
type deleteExpiredSpy struct{ calls atomic.Int32 }

func (*deleteExpiredSpy) Save(context.Context, *token.RefreshToken) error { return nil }
func (*deleteExpiredSpy) FindByHash(context.Context, string) (*token.RefreshToken, error) {
	return nil, nil
}
func (*deleteExpiredSpy) MarkUsed(context.Context, string) (bool, error) { return true, nil }
func (*deleteExpiredSpy) DeleteByUserID(context.Context, string) error   { return nil }
func (s *deleteExpiredSpy) DeleteExpired(context.Context) error {
	s.calls.Add(1)
	return nil
}

// Compile-time guards: the spies must satisfy the real domain interfaces.
var (
	_ token.TokenRepository = (*deleteExpiredSpy)(nil)
	_ shared.DomainEvent    = rowVisibleEvent{}
)
