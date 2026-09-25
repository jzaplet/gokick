//go:build !nosqlite

package testfx

import (
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/persistence"
	"gokick/app/infrastructure/sqlite"

	"github.com/ncruces/go-sqlite3"
)

// openSQLite gives the test its own database file in t.TempDir(), wired exactly
// like production (persistence.SQLiteStore); the file and its pool go away with
// the test.
func openSQLite(
	t *testing.T,
	cfg *config.Config,
	logger *slog.Logger,
) (*persistence.Store, backend) {
	t.Helper()
	cfg.DBPath = filepath.Join(t.TempDir(), "fixture.db")
	mgr, err := sqlite.NewManager(cfg)
	if err != nil {
		t.Fatalf("testfx: open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return persistence.SQLiteStore(mgr, logger), backend{
		db:        mgr.DB(),
		nowPlus:   sqlite.LeaseExpr,
		violation: sqliteViolation,
	}
}

func sqliteViolation(err error) Constraint {
	var serr *sqlite3.Error
	if !errors.As(err, &serr) {
		return ""
	}
	switch serr.ExtendedCode() {
	case sqlite3.CONSTRAINT_NOTNULL:
		return NotNull
	case sqlite3.CONSTRAINT_UNIQUE, sqlite3.CONSTRAINT_PRIMARYKEY:
		return Unique
	case sqlite3.CONSTRAINT_CHECK:
		return Check
	case sqlite3.CONSTRAINT_FOREIGNKEY:
		return ForeignKey
	default:
		return ""
	}
}
