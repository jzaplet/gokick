package console

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformcmd "gokick/app/application/platform/command"
	runapp "gokick/app/application/run"
	tenantcmd "gokick/app/application/tenant/command"
	tenantqry "gokick/app/application/tenant/query"
	usercmd "gokick/app/application/user/command"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/scheduler"
	"gokick/app/infrastructure/worker"
	"gokick/app/internal/testfx"
	"gokick/app/presentation/http/handler"
	"gokick/app/presentation/http/middleware"
	"gokick/app/presentation/http/server"

	"github.com/spf13/cobra"
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
		nil, nil, nil, nil, nil, nil, nil,
	)
}

func serveTestRunWorker(
	t *testing.T,
	fx *testfx.Fixture,
	logger *slog.Logger,
) *worker.RunWorker {
	t.Helper()
	registry, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{}, time.Second)
	if err != nil {
		t.Fatalf("run registry: %v", err)
	}
	return worker.NewRunWorker(
		logger,
		shared.NopReporter{},
		fx.Runs,
		registry,
		nil,
		nil,
		worker.RunWorkerConfig{},
	)
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

	srv := serveTestServer(logger)

	cmd := NewServeCommand(srv, sched, serveTestRunWorker(t, fx, logger)).Command()
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
		"run worker: starting",
		"run worker: stopped",
	} {
		if !contains(msgs, want) {
			t.Fatalf("missing %q in lifecycle logs — co-run/drain not observed; saw %v", want, msgs)
		}
	}
}

// The create-superadmin CLI command delegates to the platform handler and
// persists a superadmin out-of-band — the path the HTTP/admin API refuses. It
// also enforces the required flags before RunE runs.
func TestCreateSuperAdminCommand_CreatesSuperAdmin(t *testing.T) {
	// No t.Parallel — see the other testfx-backed tests in this package (goose globals).
	fx := testfx.New(t, filepath.Join(t.TempDir(), "console_superadmin.db"))
	handler := platformcmd.NewCreateSuperAdminHandler(fx.Users, fx.Hasher)

	cmd := NewCreateSuperAdminCommand(handler, fx.NewSystemBus()).Command()
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"-n", "root", "-p", "secret12"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute create-superadmin: %v", err)
	}

	got, err := fx.Users.FindByNickname(context.Background(), "root")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.Role != "superadmin" {
		t.Fatalf("create-superadmin must persist a superadmin, got %+v", got)
	}

	// The SystemCommandBus's AuditMiddleware persisted the user.created trail.
	var n int
	if err := fx.DB.DB().GetContext(context.Background(), &n,
		`SELECT COUNT(*) FROM audit_log WHERE action='user.created' AND target_id=?`, got.ID); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if n != 1 {
		t.Fatalf("create-superadmin must write a user.created audit record, got %d", n)
	}
}

func TestCreateSuperAdminCommand_MissingPasswordErrors(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "console_superadmin_missing.db"))
	handler := platformcmd.NewCreateSuperAdminHandler(fx.Users, fx.Hasher)

	cmd := NewCreateSuperAdminCommand(handler, fx.NewSystemBus()).Command()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"-n", "root"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected error when --password is missing")
	}

	if got, _ := fx.Users.FindByNickname(context.Background(), "root"); got != nil {
		t.Fatal("no user must be created when a required flag is missing")
	}
}

// newCreateUserCmd builds the create-user CLI command wired to a real fixture,
// with multitenancy on/off, for the tenant-flag matrix tests.
func newCreateUserCmd(fx *testfx.Fixture, multitenant bool) *cobra.Command {
	createUser := usercmd.NewCreateUserHandler(
		fx.Users,
		fx.Hasher,
		shared.Multitenancy(multitenant),
	)
	createTenant := tenantcmd.NewCreateTenantHandler(fx.Tenants)
	getTenant := tenantqry.NewGetTenantHandler(fx.Tenants)
	cmd := NewCreateUserCommand(
		createUser,
		createTenant,
		getTenant,
		&config.Config{Multitenancy: multitenant},
		fx.NewSystemBus(),
	).Command()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

// Multitenancy on, no tenant flag → error AND no user persisted.
func TestCreateUserCommand_MultitenantRequiresTenantFlag(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cu_mt_required.db"))

	cmd := newCreateUserCmd(fx, true)
	cmd.SetArgs([]string{"-n", "alice", "-p", "secret12"})
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		t.Fatal("multitenancy on without a tenant flag must error")
	}
	if !strings.Contains(err.Error(), "multitenancy is on") {
		t.Fatalf("error must explain the requirement, got %q", err)
	}
	if got, _ := fx.Users.FindByNickname(ctx, "alice"); got != nil {
		t.Fatal("no user must be created when the tenant flag is missing")
	}
}

// Multitenancy on + --tenant-name → creates the tenant and the user in it.
func TestCreateUserCommand_MultitenantCreatesTenantByName(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cu_mt_name.db"))

	cmd := newCreateUserCmd(fx, true)
	cmd.SetArgs([]string{"-n", "alice", "-p", "secret12", "-r", "user", "--tenant-name", "Acme"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}

	u, _ := fx.Users.FindByNickname(ctx, "alice")
	if u == nil {
		t.Fatal("user must be created")
	}
	tn, _ := fx.Tenants.FindByID(ctx, u.TenantID)
	if tn == nil || tn.Name != "Acme" {
		t.Fatalf("user must land in the new tenant 'Acme', got %+v", tn)
	}
}

// Multitenancy on + --tenant-id to an existing tenant → the user lands in it.
func TestCreateUserCommand_MultitenantUsesExistingTenantId(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cu_mt_id.db"))
	tn := fx.SeedTenant(t, "Beta")

	cmd := newCreateUserCmd(fx, true)
	cmd.SetArgs([]string{"-n", "bob", "-p", "secret12", "-r", "user", "--tenant-id", tn.ID})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}

	u, _ := fx.Users.FindByNickname(ctx, "bob")
	if u == nil || u.TenantID != tn.ID {
		t.Fatalf("user must land in tenant %q, got %+v", tn.ID, u)
	}
}

// Unknown --tenant-id → clean "not found", no user, no orphan tenant.
func TestCreateUserCommand_MultitenantUnknownTenantIdErrors(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cu_mt_badid.db"))

	cmd := newCreateUserCmd(fx, true)
	cmd.SetArgs([]string{"-n", "alice", "-p", "secret12", "--tenant-id", "no-such-id"})
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		t.Fatal("an unknown --tenant-id must error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error must say the tenant was not found, got %q", err)
	}
	if got, _ := fx.Users.FindByNickname(ctx, "alice"); got != nil {
		t.Fatal("no user must be created for an unknown tenant")
	}
}

// Multitenancy off + a tenant flag → error (the flags are not applicable).
func TestCreateUserCommand_SingleTenantRejectsTenantFlag(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cu_single_flag.db"))

	cmd := newCreateUserCmd(fx, false)
	cmd.SetArgs([]string{"-n", "alice", "-p", "secret12", "--tenant-name", "Acme"})
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		t.Fatal("multitenancy off with a tenant flag must error")
	}
	if !strings.Contains(err.Error(), "multitenancy is off") {
		t.Fatalf("error must explain, got %q", err)
	}
}

// create-tenant prints the new tenant and persists it.
func TestCreateTenantCommand_CreatesTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ct_create.db"))

	cmd := NewCreateTenantCommand(
		tenantcmd.NewCreateTenantHandler(fx.Tenants),
		fx.NewSystemBus(),
	).Command()
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"-n", "Acme"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}

	tn, _ := fx.Tenants.FindByName(ctx, "Acme")
	if tn == nil {
		t.Fatal("create-tenant must persist the tenant")
	}

	// The SystemCommandBus's AuditMiddleware persisted the tenant.created trail.
	var n int
	if err := fx.DB.DB().GetContext(ctx, &n,
		`SELECT COUNT(*) FROM audit_log WHERE action='tenant.created' AND target_id=?`, tn.ID); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if n != 1 {
		t.Fatalf("create-tenant must write a tenant.created audit record, got %d", n)
	}
}

// Rollback: --tenant-name creates a tenant before the user; if user creation
// fails (here: duplicate nickname), the just-created tenant must be rolled back —
// no orphan. create-user dispatches through the SystemCommandBus, so its
// TransactionMiddleware wraps tenant resolution + user creation in one tx.
func TestCreateUserCommand_TenantNameRolledBackWhenUserFails(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cu_rollback.db"))

	// A user named "alice" already exists → create-user with the same nickname fails.
	fx.SeedUserInTenant(t, "alice", "user", shared.DefaultTenantID)

	cmd := newCreateUserCmd(fx, true)
	cmd.SetArgs([]string{"-n", "alice", "-p", "secret12", "-r", "user", "--tenant-name", "Beta"})
	if err := cmd.ExecuteContext(ctx); err == nil {
		t.Fatal("create-user with a duplicate nickname must fail")
	}

	// The tenant 'Beta' must NOT exist — the failed user creation rolled it back.
	if tn, _ := fx.Tenants.FindByName(ctx, "Beta"); tn != nil {
		t.Fatalf("orphan tenant: 'Beta' must be rolled back when user creation fails, got %+v", tn)
	}
}
