package testfx

import (
	"errors"
	"log/slog"
	"testing"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/persistence"
	"gokick/app/infrastructure/postgres"
	"gokick/app/internal/testfx/pgfx"

	"github.com/jackc/pgx/v5/pgconn"
)

// openPostgres gives the test its own database — a clone of the migrated
// template (pgfx), dropped when the test ends — wired exactly like production
// (persistence.PostgresStore) over the two role pools. The fixture's own writes
// go through the system role, which bypasses row-level security: a fixture sets
// up any tenant's rows.
func openPostgres(
	t *testing.T,
	cfg *config.Config,
	logger *slog.Logger,
) (*persistence.Store, backend) {
	t.Helper()
	db := pgfx.New(t).Config()
	cfg.DBURL, cfg.DBSystemURL, cfg.DBMigrateURL = db.DBURL, db.DBSystemURL, db.DBMigrateURL
	cfg.DBLockTimeout, cfg.DBStatementTimeout, cfg.DBIdleTxTimeout =
		db.DBLockTimeout, db.DBStatementTimeout, db.DBIdleTxTimeout
	mgr, err := postgres.NewManager(cfg)
	if err != nil {
		t.Fatalf("testfx: open postgres: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return persistence.PostgresStore(mgr, cfg.DBMigrateURL, logger), backend{
		db:        mgr.System(),
		nowPlus:   postgres.NowPlus("?"),
		violation: postgresViolation,
	}
}

func postgresViolation(err error) Constraint {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	switch pgErr.Code {
	case "23502":
		return NotNull
	case "23505":
		return Unique
	case "23514":
		return Check
	case "23503":
		return ForeignKey
	default:
		return ""
	}
}
