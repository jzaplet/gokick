//go:build !nosqlite

package sqlite

import (
	"context"
	"database/sql"
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

// Migrator applies the embedded SQLite migration set (migrations.SQLite) — the
// adapter's database.Migrator.
type Migrator struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewMigrator(manager *Manager, logger *slog.Logger) *Migrator {
	return &Migrator{
		db:     manager.DB(),
		logger: logger,
	}
}

// NewMigrationProvider builds the goose provider for the SQLite migration
// set. A Provider carries its own state (no package-level goose globals), so two
// migrators — or two tests — never share a dialect/FS setting. The global Go
// migration registry is disabled: this project ships SQL migrations only.
//
// A Provider runs every SQL migration of a run on ONE pinned *sql.Conn, including
// `-- +goose NO TRANSACTION` ones. That is what SQLite table-rebuild migrations
// need: they toggle the per-connection PRAGMA foreign_keys (so dropping a
// referenced table does not cascade-delete through ON DELETE CASCADE), and the
// PRAGMA must hold for the whole rebuild. The pool itself is never narrowed, so
// its cap (F-047) is untouched.
func NewMigrationProvider(db *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectSQLite3, db, migrations.SQLite,
		goose.WithDisableGlobalRegistry(true),
		goose.WithLogger(goose.NopLogger()),
	)
}

func (m *Migrator) RunUp() error {
	ctx := context.Background()
	provider, err := NewMigrationProvider(m.db.DB)
	if err != nil {
		return err
	}

	before, errBefore := provider.GetDBVersion(ctx)

	if _, err := provider.Up(ctx); err != nil {
		return err
	}

	after, errAfter := provider.GetDBVersion(ctx)

	switch {
	case errBefore != nil || errAfter != nil:
		// A version read failed. The migrations themselves succeeded (Up
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
