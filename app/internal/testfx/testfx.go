// Package testfx provides shared test fixtures for application-layer handlers.
// Every fixture gets its own freshly migrated database on the adapter the run
// targets (APP_DB_DRIVER — see ActiveDriver), wired through the same
// persistence Store as production, plus real implementations of the common
// dependencies (password hasher, JWT, repositories). Nothing in a test names the
// database: the same test runs unchanged on every adapter.
package testfx

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"gokick/app/application/bus"
	busmw "gokick/app/application/bus/middleware"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/persistence"
	"gokick/app/infrastructure/security"
	"gokick/app/internal/testfx/jwtfx"

	"github.com/jmoiron/sqlx"
)

type Fixture struct {
	// Tx opens and ends transactions on the fixture database — the same
	// shared.Transactor the bus TransactionMiddleware uses.
	Tx              shared.Transactor
	Users           user.Repository
	PlatformUsers   user.PlatformRepository // same concrete repo; the cross-tenant port for platform handler tests
	Tokens          token.Repository
	Runs            run.Repository
	Tenants         tenant.Repository
	PlatformTenants tenant.PlatformRepository // same concrete repo; cross-tenant port for platform tests
	// Audit is the real AuditLogger (raw pool — survives a business rollback).
	Audit  shared.AuditLogger
	Hasher *security.PasswordHasher
	Jwt    *security.JwtService

	backend
}

// backend is what an adapter's fixture opener hands back: the production Store
// plus the handle and dialect bits the fixture helpers in raw.go need.
type backend struct {
	store *persistence.Store
	// db is the fixture handle for writes that reach past the repositories.
	db *sqlx.DB
	// nowPlus is a SQL expression for the database clock shifted by ? seconds,
	// in the adapter's timestamp encoding.
	nowPlus string
	// violation classifies a driver error by the constraint it broke.
	violation func(error) Constraint
}

// New builds a single-tenant fixture (APP_MULTITENANCY off — the default). The
// database is dropped automatically when the test completes.
func New(t *testing.T) *Fixture { return newFixture(t, false) }

// NewMultitenant builds a fixture with multitenant enforcement ON (fail-closed):
// a query whose context carries no tenant panics instead of falling back to the
// default tenant. Use it to assert the fail-closed guard.
func NewMultitenant(t *testing.T) *Fixture { return newFixture(t, true) }

func newFixture(t *testing.T, multitenant bool) *Fixture {
	t.Helper()

	cfg := &config.Config{
		DBDriver:             ActiveDriver(),
		JWTSecret:            jwtfx.Secret,
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
		Multitenancy:         multitenant,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var b backend
	switch cfg.DBDriver {
	case database.DriverSQLite:
		b = openSQLite(t, cfg, logger)
	case database.DriverPostgres:
		// Phase 4 of the Postgres adapter plan adds the repositories, and with them
		// this fixture. Until then only the adapter's own tests run on Postgres.
		t.Fatal("testfx: the Postgres adapter has no repositories yet, so no fixture " +
			"can be built on it — only its own tests run on postgres (make test-pg)")
	default:
		t.Fatalf("testfx: no fixture backend for APP_DB_DRIVER=%s in this build", cfg.DBDriver)
	}
	if err := b.store.Migrator.RunUp(); err != nil {
		t.Fatalf("testfx: migrate: %v", err)
	}

	jwt, err := security.NewJwtService(cfg)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}

	return &Fixture{
		Tx:              b.store.Tx,
		Users:           b.store.Users,
		PlatformUsers:   b.store.PlatformUsers,
		Tokens:          b.store.Tokens,
		Runs:            b.store.Runs,
		Tenants:         b.store.Tenants,
		PlatformTenants: b.store.PlatformTenants,
		Audit:           b.store.Audit,
		Hasher:          security.NewPasswordHasher(),
		Jwt:             jwt,
		backend:         b,
	}
}

// SystemCtx is the context the seed helpers below write with: the system plane,
// outside any tenant. Seeding is setup, not tenant work — on Postgres the tenant
// plane's row-level security would refuse a row stamped with any tenant but the
// active one, and the helpers seed every tenant. Use it for any other fixture
// write that must reach past a tenant.
func SystemCtx() context.Context {
	return shared.ContextWithPlane(context.Background(), shared.PlaneSystem)
}

// HashToken returns the SHA-256 hex hash of the raw refresh token.
func (*Fixture) HashToken(raw string) string {
	return security.HashToken(raw)
}

// NewBuses wires a production-like CommandBus + QueryBus + EventBus mirroring
// what container_provider builds (logger silent via io.Discard). Tests that
// need to inspect collected events should use shared.ContextWithEventCollector
// directly when invoking a handler outside the bus.
func (f *Fixture) NewBuses() (*bus.CommandBus, *bus.QueryBus, *bus.EventBus) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := security.NewPermissionChecker()
	resolver := security.NewDefaultTenantResolver()
	reporter := shared.NopReporter{}

	eventBus := bus.NewEventBus(
		busmw.RecoveryMiddleware(logger, reporter),
		busmw.LoggingMiddleware(logger),
	)

	// Throwaway run dispatcher (the no-op) — no command handler enqueues today, so
	// RunDispatcherMiddleware injects it but it is never invoked. Importing the real
	// application/run dispatcher here would cycle (its test imports testfx). The chain
	// stays faithful via CommandChain.
	runDispatcher := shared.RunDispatcherFromContext(context.Background())

	// Same chain as provideCommandBus (busmw.CommandChain is the single source),
	// so the test CommandBus can't drift from production — incl. Audit + RunDispatcher.
	return bus.NewCommandBus(
			busmw.CommandChain(
				logger,
				checker,
				reporter,
				resolver,
				f.Audit,
				runDispatcher,
				eventBus,
				f.Tx,
			)...,
		),
		bus.NewQueryBus(busmw.QueryChain(logger, checker, reporter, resolver, f.Tx)...),
		eventBus
}

// NewSystemBus wires a SystemCommandBus for CLI-command tests. It uses the SAME
// busmw.SystemChain as provideSystemCommandBus (the single source of the chain),
// so the test bus can never drift from production — add a middleware once and
// both get it. Audit writes land in the real audit_log table.
func (f *Fixture) NewSystemBus() *bus.SystemCommandBus {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reporter := shared.NopReporter{}

	eventBus := bus.NewEventBus(
		busmw.RecoveryMiddleware(logger, reporter),
		busmw.LoggingMiddleware(logger),
	)

	// Throwaway no-op run dispatcher, same as NewBuses: no CLI command's event
	// handler enqueues today, and importing the real application/run dispatcher
	// here would cycle (its test imports testfx). A test that needs a real enqueue
	// via the system bus builds its own SystemChain with a live dispatcher.
	runDispatcher := shared.RunDispatcherFromContext(context.Background())

	return bus.NewSystemCommandBus(
		busmw.SystemChain(
			logger, f.Tx, eventBus, f.Audit, runDispatcher, reporter,
		)...,
	)
}

// ExecCommand dispatches cmd through cmdBus to handlerFn and returns the
// handler's typed result. Use this in handler tests that need the full
// middleware chain (tx, audit, events, …) wrapped around a call —
// importing `application/bus` directly from a handler package would
// violate the arch-lint rule that `application` components depend on
// `bus_middleware` only, not the bus itself. testfx is the sanctioned
// escape hatch (it already wires the bus for fixtures).
func ExecCommand[R any](
	ctx context.Context,
	cmdBus *bus.CommandBus,
	name string,
	cmd any,
	handlerFn func(ctx context.Context) (R, error),
) (R, error) {
	return bus.Dispatch(ctx, cmdBus, name, cmd, handlerFn)
}

// ExecQuery is ExecCommand's read-side twin: it dispatches q through queryBus so
// a query handler test runs the full read chain (recovery, logging, authorize,
// tenant). Same arch-lint rationale — application packages can't import `bus`
// directly, so they go through this fixture helper.
func ExecQuery[R any](
	ctx context.Context,
	queryBus *bus.QueryBus,
	name string,
	q any,
	handlerFn func(ctx context.Context) (R, error),
) (R, error) {
	return bus.Query(ctx, queryBus, name, q, handlerFn)
}

// AssertTokenCount fails the test if the refresh_tokens row count differs from want.
func (f *Fixture) AssertTokenCount(t *testing.T, want int) {
	t.Helper()
	if got := f.Count(t, "refresh_tokens", ""); got != want {
		t.Fatalf("refresh_tokens count: got %d want %d", got, want)
	}
}

// SeedUser persists a user with the given nickname/password/role and returns the entity.
func (f *Fixture) SeedUser(t *testing.T, nickname, password, role string) *user.User {
	t.Helper()
	hash, err := f.Hasher.Hash(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	nn, err := user.NewNickname(nickname)
	if err != nil {
		t.Fatalf("nickname: %v", err)
	}
	r, err := user.NewRole(role)
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	em, err := user.NewEmail(nickname + "@example.com")
	if err != nil {
		t.Fatalf("email: %v", err)
	}
	u := user.NewUser(nn, hash, em, r, shared.DefaultTenantID)
	if err := f.Users.Save(SystemCtx(), u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return u
}

// SeedTenant persists a tenant with the given name and returns it. Used by
// multitenant tests to create the distinct tenants whose isolation they assert.
func (f *Fixture) SeedTenant(t *testing.T, name string) *tenant.Tenant {
	t.Helper()
	n, err := tenant.NewName(name)
	if err != nil {
		t.Fatalf("tenant name: %v", err)
	}
	tn := tenant.NewTenant(n)
	if err := f.Tenants.Save(SystemCtx(), tn); err != nil {
		t.Fatalf("save tenant: %v", err)
	}
	return tn
}

// SeedTenantWithPlan persists a tenant on a non-default billing tier. NewTenant
// always stamps PlanFree (gokick ships only that tier), so a test that needs the
// plan column to actually vary — the tenants grid filters on it — sets it here
// rather than reaching into the DB.
func (f *Fixture) SeedTenantWithPlan(t *testing.T, name, plan string) *tenant.Tenant {
	t.Helper()
	tn := f.SeedTenant(t, name)
	tn.Plan = plan
	f.setTenantPlan(t, tn.ID, plan)
	return tn
}

// SeedUserInTenant persists a user stamped with the given tenant id — used by
// isolation tests to populate distinct tenants.
func (f *Fixture) SeedUserInTenant(t *testing.T, nickname, role, tenantID string) *user.User {
	t.Helper()
	hash, err := f.Hasher.Hash("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	nn, err := user.NewNickname(nickname)
	if err != nil {
		t.Fatalf("nickname: %v", err)
	}
	r, err := user.NewRole(role)
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	em, err := user.NewEmail(nickname + "@example.com")
	if err != nil {
		t.Fatalf("email: %v", err)
	}
	u := user.NewUser(nn, hash, em, r, tenantID)
	if err := f.Users.Save(SystemCtx(), u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return u
}

// SeedRunInTenant enqueues a PENDING run stamped with the given tenant id — the
// state a tenant-owned background task sits in between being enqueued and being
// claimed. Used by tests that assert what a tenant still owns besides its users.
//
// It stamps TenantID onto the row directly rather than going through the
// dispatcher: the dispatcher reads the tenant off ctx and refuses an unregistered
// kind, and a test asserting the tenant-delete gate wants neither a bus nor a
// handler registry — just a row that exists.
func (f *Fixture) SeedRunInTenant(t *testing.T, kind, tenantID string) *run.Run {
	t.Helper()
	r, err := run.NewRun(kind, []byte(`{}`), 0)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	r.TenantID = tenantID
	if err := f.Runs.Enqueue(SystemCtx(), r); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}
	return r
}

// MarkRunCompleted drives a seeded run to the terminal completed state through the
// real fenced path — claim it, then finalize as its owner. Faking completed_at with
// raw SQL would let the fixture disagree with what the repository considers
// terminal, which is the exact drift these tests exist to catch.
func (f *Fixture) MarkRunCompleted(t *testing.T, id string) {
	t.Helper()
	ctx := SystemCtx()
	const owner = "testfx-owner"

	claimed, err := f.Runs.ClaimDue(ctx, owner, time.Minute)
	if err != nil {
		t.Fatalf("claim run: %v", err)
	}
	if claimed == nil {
		t.Fatal("no run was due to claim — seed one first")
	}
	if claimed.ID != id {
		t.Fatalf("claimed a different run (%s, want %s) — seed only the run you finalize",
			claimed.ID, id)
	}
	ok, err := f.Runs.MarkComplete(ctx, id, owner)
	if err != nil {
		t.Fatalf("mark complete: %v", err)
	}
	if !ok {
		t.Fatal("MarkComplete affected no rows — the lease was lost or the run was terminal")
	}
}

// SeedRefreshToken persists a refresh token for the user and returns the raw (unhashed) value.
func (f *Fixture) SeedRefreshToken(t *testing.T, userID string, expiresAt time.Time) string {
	t.Helper()
	raw, hash, _, err := f.Jwt.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("generate refresh: %v", err)
	}
	rt := token.NewRefreshToken(userID, hash, expiresAt)
	if err := f.Tokens.Save(SystemCtx(), rt); err != nil {
		t.Fatalf("save token: %v", err)
	}
	return raw
}
