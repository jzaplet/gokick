package console

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	jobapp "gokick/app/application/job"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/scheduler"
	"gokick/app/infrastructure/worker"
	"gokick/app/internal/testfx"
	"gokick/app/presentation/http/handler"
	"gokick/app/presentation/http/middleware"
	"gokick/app/presentation/http/server"
)

// ---------------------------------------------------------------------------
// serve command lifecycle — scheduler + worker co-run, share one ctx, and RunE
// drains both before returning.
//
//	roadmap-20         — scheduler.Run(ctx) launched in a goroutine before
//	                     server.Start(ctx); a single ctx cancel drains it.
//	roadmap-21         — the schedulerDone channel gates RunE: it does NOT
//	                     return until the scheduler goroutine has drained.
//	presentation-03    — scheduler and HTTP server share one ctx (the cancel
//	                     that stops the server also stops the scheduler).
//	infra-sched-job-11 — scheduler runs in-process on the server lifecycle;
//	                     one ctx-cancel drains both.
//	roadmap-41 (serve) — serve also co-runs an in-process worker (alongside the
//	                     standalone `worker` command).
//	overview-105       — serve starts the HTTP server (server.Start is invoked
//	                     and owns the blocking lifecycle).
// ---------------------------------------------------------------------------

// captureHandler is a slog.Handler that records the message of every emitted
// record. Used to prove the scheduler and worker actually ran (and stopped)
// within serve's lifecycle, since both are concrete types with no other seam.
type captureHandler struct {
	mu   *sync.Mutex
	msgs *[]string
}

func newCaptureLogger() (*slog.Logger, func() []string) {
	mu := &sync.Mutex{}
	msgs := &[]string{}
	h := captureHandler{mu: mu, msgs: msgs}
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(*msgs))
		copy(out, *msgs)
		return out
	}
	return slog.New(h), snapshot
}

func (captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.msgs = append(*h.msgs, r.Message)
	return nil
}

func (h captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h captureHandler) WithGroup(string) slog.Handler      { return h }

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// serveTestServer builds a *server.Server with the minimal real dependencies
// Start() touches: config (port :0 → OS-assigned free port), a capturing
// logger, an IP extractor, and the two rate limiters whose janitors Start
// spawns. The bus-backed route handlers stay nil — no request is ever served
// (ctx is cancelled before the listener takes traffic), so registerRoutes only
// forms method values from the nil handlers (safe) and never dereferences them.
func serveTestServer(logger *slog.Logger) *server.Server {
	extract := middleware.NewIPExtractor(false)
	rule := middleware.RateRule{Tokens: 1000, Per: time.Minute}
	limiters := &server.RateLimiters{
		Login:   middleware.NewRateLimiter(rule, extract, logger),
		Refresh: middleware.NewRateLimiter(rule, extract, logger),
	}
	return server.NewServer(
		&config.Config{HTTPPort: "0", CookieSecure: false, CORSOrigin: "*"},
		logger,
		shared.NopReporter{},
		nil, // jwt — only used by registerRoutes' AuthMiddleware wrapper, never invoked
		limiters,
		extract,
		handler.NewHealthHandler(),
		nil, nil, nil, nil, nil,
	)
}

// serveTestWorker builds a real Worker (empty handler registry, throwaway
// dispatcher) the same cheap way newTestWorker does, but routed through the
// supplied logger so its "worker: starting" / "worker: stopped" lifecycle
// lines land in the capture buffer. Its Run drains promptly on ctx cancel.
func serveTestWorker(t *testing.T, fx *testfx.Fixture, logger *slog.Logger) *worker.Worker {
	t.Helper()
	registry, err := jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	dispatcher := shared.JobDispatcherFromContext(context.Background())
	return worker.NewWorker(logger, shared.NopReporter{}, fx.Jobs, registry, fx.DB, dispatcher, 1)
}

// TestServeCommand_SchedulerDoneGatesReturnAndSharesCtx is the load-bearing
// drain test for serve's RunE. It proves, deterministically, that:
//
//   - the scheduler is launched and shares serve's ctx — cancelling the ctx
//     that stops the server also unblocks the scheduler job (roadmap-20,
//     presentation-03, infra-sched-job-11);
//   - RunE blocks on <-schedulerDone and will NOT return while the scheduler
//     goroutine is still draining (roadmap-21);
//   - the worker co-runs and is drained within the same lifecycle (roadmap-41);
//   - server.Start owns the blocking lifecycle (overview-105).
//
// Determinism: the scheduler job blocks on <-ctx.Done(), then on <-release
// before returning, so scheduler.Run cannot finish (and schedulerDone cannot
// close) until the test closes release. The test asserts RunE is still blocked
// at that point, then closes release and asserts RunE returns.
func TestServeCommand_SchedulerDoneGatesReturnAndSharesCtx(t *testing.T) {
	// No t.Parallel: testfx.New runs goose migrations (process-global SetLogger/
	// SetDialect) — concurrent New() calls race under -race. Matches the rest of
	// the testfx-backed tests in this package.
	fx := testfx.New(t, filepath.Join(t.TempDir(), "serve.db"))

	logger, snapshot := newCaptureLogger()

	schedStarted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(closeRelease) // never leak the scheduler goroutine if an assert fails

	job := scheduler.Job{
		Name:     "blocking-maintenance",
		Interval: time.Hour, // large: the run-once invocation is the only tick
		Fn: func(ctx context.Context) error {
			close(schedStarted)
			<-ctx.Done() // shares serve's ctx — unblocks on the same cancel as the server
			<-release    // gate: scheduler.Run can't finish until the test allows it
			return nil
		},
	}
	sched, err := scheduler.NewScheduler(logger, []scheduler.Job{job})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	w := serveTestWorker(t, fx, logger)
	srv := serveTestServer(logger)

	cmd := NewServeCommand(srv, sched, w).Command()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)

	runReturned := make(chan error, 1)
	go func() { runReturned <- cmd.RunE(cmd, nil) }()

	// Wait for the scheduler job to actually start (proves it was launched).
	select {
	case <-schedStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler job never started — serve did not launch scheduler.Run")
	}

	// Cancel serve's ctx. server.Start drains and returns; the scheduler job
	// unblocks from <-ctx.Done() but then parks on <-release. RunE must now be
	// blocked on <-schedulerDone.
	cancel()

	// roadmap-21: RunE must NOT have returned — the scheduler hasn't drained.
	select {
	case err := <-runReturned:
		t.Fatalf(
			"RunE returned before the scheduler drained (schedulerDone not awaited): err=%v",
			err,
		)
	case <-time.After(150 * time.Millisecond):
	}

	// Let the scheduler finish. Now schedulerDone closes and RunE can return.
	closeRelease()

	select {
	case err := <-runReturned:
		if err != nil {
			t.Fatalf("serve RunE returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunE did not return after the scheduler drained")
	}

	// Both the scheduler and the worker must have co-run AND stopped within
	// serve's lifecycle (roadmap-41 worker half, infra-sched-job-11). The
	// "stopped" lines are emitted only after each Run() drains.
	msgs := snapshot()
	for _, want := range []string{
		"scheduler: starting",
		"scheduler: stopped",
		"worker: starting",
		"worker: stopped",
	} {
		if !contains(msgs, want) {
			t.Fatalf("missing %q in lifecycle logs — co-run/drain not observed; saw %v", want, msgs)
		}
	}
}
