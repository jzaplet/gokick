package postgres_test

import (
	"context"
	"errors"
	"testing"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
	"gokick/app/internal/testfx/pgfx"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// SQLSTATE codes the tests assert on.
const (
	codeInsufficientPrivilege = "42501" // no grant, or a row-level security WITH CHECK
	codeReadOnlyTransaction   = "25006"
	codeUniqueViolation       = "23505"
)

// newManager opens a Manager over cfg and closes it with the test.
func newManager(t *testing.T, cfg *config.Config) *postgres.Manager {
	t.Helper()
	mgr, err := postgres.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

// migrated returns a fresh migrated database and a Manager over it.
func migrated(t *testing.T) (*pgfx.DB, *postgres.Manager) {
	t.Helper()
	db := pgfx.New(t)
	return db, newManager(t, db.Config())
}

// inTx runs fn in a transaction the Manager opens for ctx (its plane and tenant),
// then rolls it back.
func inTx(t *testing.T, mgr *postgres.Manager, ctx context.Context, fn func(tx *sqlx.Tx)) {
	t.Helper()
	txCtx, err := mgr.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = mgr.Rollback(txCtx) }()
	fn(database.TxFromContext(txCtx))
}

// sqlState returns the SQLSTATE of a Postgres error, or "" for anything else.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }
