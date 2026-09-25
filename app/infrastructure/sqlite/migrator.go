//go:build !nosqlite

package sqlite

import (
	"context"
	"database/sql"
	"gokick/app/infrastructure/database"
	"gokick/migrations"
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
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
	provider, err := NewMigrationProvider(m.db.DB)
	if err != nil {
		return err
	}
	return database.MigrateUp(context.Background(), provider, m.logger)
}
