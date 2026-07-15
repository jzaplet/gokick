package database

import (
	"context"
	"errors"
	"gokick/app/domain/shared"
	"gokick/migrations"
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

// Migration-local structured-log keys. sloglint's no-raw-keys forbids bare
// string keys.
const (
	logKeyFrom    = "from"
	logKeyTo      = "to"
	logKeyVersion = "version"
)

type MigrationManager struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewMigrationManager(manager *SqliteManager, logger *slog.Logger) *MigrationManager {
	return &MigrationManager{
		db:     manager.DB(),
		logger: logger,
	}
}

func (m *MigrationManager) RunUp() error {
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}

	// Pin migrations to a single connection. SQLite table-rebuild migrations use
	// `-- +goose NO TRANSACTION` so they can toggle PRAGMA foreign_keys (needed
	// so a DROP of a referenced table doesn't cascade-delete via ON DELETE
	// CASCADE) — but PRAGMA is per-connection, and goose runs NO TRANSACTION
	// statements on the *sql.DB pool, where the next statement may land on a
	// different connection. One connection makes the PRAGMA hold across the whole
	// rebuild. Migrations run once at startup, serially, so this costs nothing;
	// restore the pool afterward.
	m.db.SetMaxOpenConns(1)
	defer m.db.SetMaxOpenConns(0)

	before, errBefore := goose.GetDBVersion(m.db.DB)

	if err := goose.UpContext(context.Background(), m.db.DB, "."); err != nil {
		return err
	}

	after, errAfter := goose.GetDBVersion(m.db.DB)

	switch {
	case errBefore != nil || errAfter != nil:
		// A version read failed. The migrations themselves succeeded (UpContext
		// returned nil), so this is a reporting-only degradation — but don't
		// fabricate an applied-range from a swallowed 0 (that would log a phantom
		// "0 -> N" or "up to date version 0"). Surface the read failure instead.
		m.logger.Warn("migrations: applied, but version read failed (applied-range log skipped)",
			shared.LogKeyError, errors.Join(errBefore, errAfter))
	case after > before:
		m.logger.Info("migrations: applied", logKeyFrom, before, logKeyTo, after)
	default:
		m.logger.Info("migrations: up to date", logKeyVersion, after)
	}

	return nil
}
