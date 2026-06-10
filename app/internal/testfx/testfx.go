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
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/security"
	sqlitejob "gokick/app/infrastructure/sqlite/job"
	sqlitetoken "gokick/app/infrastructure/sqlite/token"
	sqliteuser "gokick/app/infrastructure/sqlite/user"

	"github.com/google/uuid"
)

type Fixture struct {
	DB     *database.SqliteManager
	Users  user.Repository
	Tokens token.TokenRepository
	Jobs   job.Repository
	Hasher *security.PasswordHasher
	Jwt    *security.JwtService
}

// New spins up an isolated SQLite database at dbPath, runs migrations and wires
// real implementations of all auth dependencies. The DB is closed automatically
// when the test completes.
func New(t *testing.T, dbPath string) *Fixture {
	t.Helper()

	cfg := &config.Config{
		DBPath:               dbPath,
		JWTSecret:            "test-secret-32-chars-long-enough",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
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

	return &Fixture{
		DB:     db,
		Users:  sqliteuser.NewRepository(db),
		Tokens: sqlitetoken.NewRepository(db),
		Jobs:   sqlitejob.NewRepository(db),
		Hasher: security.NewPasswordHasher(),
		Jwt:    jwt,
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
	reporter := shared.NopReporter{}

	eventBus := bus.NewEventBus(
		busmw.RecoveryMiddleware(logger, reporter),
		busmw.LoggingMiddleware(logger),
	)

	commandChain := append(busmw.BaseChain(logger, checker, reporter),
		busmw.DispatchEventsMiddleware(logger, eventBus),
		busmw.TransactionMiddleware(f.DB),
	)

	return bus.NewCommandBus(commandChain...),
		bus.NewQueryBus(busmw.BaseChain(logger, checker, reporter)...),
		eventBus
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
	u := user.NewUser(nn, hash, em, r)
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
