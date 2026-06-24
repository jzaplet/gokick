// Package testfx provides shared test fixtures for application-layer handlers.
// Spins up a real SQLite database with migrations and wires real implementations
// of all common dependencies (password hasher, JWT, repositories).
package testfx

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"gokick/app/application/bus"
	busmw "gokick/app/application/bus/middleware"
	"gokick/app/domain/job"
	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/security"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	sqlitejob "gokick/app/infrastructure/sqlite/job"
	sqlitetenant "gokick/app/infrastructure/sqlite/tenant"
	sqlitetoken "gokick/app/infrastructure/sqlite/token"
	sqliteuser "gokick/app/infrastructure/sqlite/user"

	"github.com/google/uuid"
)

type Fixture struct {
	DB            *database.SqliteManager
	Users         user.Repository
	PlatformUsers user.PlatformRepository // same concrete repo; the cross-tenant port for platform handler tests
	Tokens        token.TokenRepository
	Jobs          job.Repository
	Tenants       tenant.Repository
	Hasher        *security.PasswordHasher
	Jwt           *security.JwtService
}

// New spins up an isolated SQLite database at dbPath, runs migrations and wires
// real implementations of all auth dependencies. The DB is closed automatically
// when the test completes.
// New builds a single-tenant fixture (APP_MULTITENANCY off — the default).
func New(t *testing.T, dbPath string) *Fixture { return newFixture(t, dbPath, false) }

// NewMultitenant builds a fixture with multitenant enforcement ON (fail-closed):
// a query whose context carries no tenant panics instead of falling back to the
// default tenant. Use it to assert the fail-closed guard.
func NewMultitenant(t *testing.T, dbPath string) *Fixture {
	return newFixture(t, dbPath, true)
}

func newFixture(t *testing.T, dbPath string, multitenant bool) *Fixture {
	t.Helper()

	cfg := &config.Config{
		DBPath:               dbPath,
		JWTSecret:            "test-secret-32-chars-long-enough",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
		Multitenancy:         multitenant,
	}

	db, err := database.NewSqliteManager(cfg)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.NewMigrationManager(db, logger).RunUp(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	jwt, err := security.NewJwtService(cfg)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}

	usersRepo := sqliteuser.NewRepository(db)

	return &Fixture{
		DB:            db,
		Users:         usersRepo,
		PlatformUsers: usersRepo,
		Tokens:        sqlitetoken.NewRepository(db),
		Jobs:          sqlitejob.NewRepository(db),
		Tenants:       sqlitetenant.NewRepository(db),
		Hasher:        security.NewPasswordHasher(),
		Jwt:           jwt,
	}
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

	// Throwaway dispatcher (the no-op the worker tests use) — no command handler
	// enqueues today, so JobDispatcherMiddleware injects it but it is never
	// invoked. Importing the real application/job dispatcher here would cycle
	// (its test imports testfx). The chain itself stays faithful via CommandChain.
	dispatcher := shared.JobDispatcherFromContext(context.Background())
	audit := sqliteaudit.NewRepository(f.DB)

	// Same chain as provideCommandBus (busmw.CommandChain is the single source),
	// so the test CommandBus can't drift from production — incl. Audit + JobDispatcher.
	return bus.NewCommandBus(
			busmw.CommandChain(
				logger,
				checker,
				reporter,
				resolver,
				audit,
				dispatcher,
				eventBus,
				f.DB,
			)...,
		),
		bus.NewQueryBus(busmw.BaseChain(logger, checker, reporter, resolver)...),
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

	return bus.NewSystemCommandBus(
		busmw.SystemChain(logger, f.DB, eventBus, sqliteaudit.NewRepository(f.DB), reporter)...,
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
	return bus.Exec(ctx, cmdBus.Bus, name, cmd, handlerFn)
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
	return bus.Exec(ctx, queryBus.Bus, name, q, handlerFn)
}

// NewJwt returns a JwtService configured with the given access expiration.
// Use when a test needs only JWT (no DB) or a non-default access expiry
// (e.g. negative duration for expired-token scenarios).
func NewJwt(t *testing.T, accessExp time.Duration) *security.JwtService {
	t.Helper()
	svc, err := security.NewJwtService(&config.Config{
		JWTSecret:            "test-secret-32-chars-long-enough",
		JWTAccessExpiration:  accessExp,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}

	return svc
}

// AssertTokenCount fails the test if the refresh_tokens row count differs from want.
func (f *Fixture) AssertTokenCount(t *testing.T, want int) {
	t.Helper()
	var got int
	if err := f.DB.DB().GetContext(context.Background(), &got, `SELECT COUNT(*) FROM refresh_tokens`); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if got != want {
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
	if err := f.Users.Save(context.Background(), u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return u
}

// SeedTenant persists a tenant with the given name and returns it. Used by
// multitenant tests to create the distinct tenants whose isolation they assert.
func (f *Fixture) SeedTenant(t *testing.T, name string) *tenant.Tenant {
	t.Helper()
	tn := tenant.NewTenant(name)
	if err := f.Tenants.Save(context.Background(), tn); err != nil {
		t.Fatalf("save tenant: %v", err)
	}
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
	if err := f.Users.Save(context.Background(), u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return u
}

// SeedRefreshToken persists a refresh token for the user and returns the raw (unhashed) value.
func (f *Fixture) SeedRefreshToken(t *testing.T, userID string, expiresAt time.Time) string {
	t.Helper()
	raw, hash, _, err := f.Jwt.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("generate refresh: %v", err)
	}
	rt := &token.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	if err := f.Tokens.Save(context.Background(), rt); err != nil {
		t.Fatalf("save token: %v", err)
	}
	return raw
}
