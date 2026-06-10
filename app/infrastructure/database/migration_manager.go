package database

import (
	"context"
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

	before, _ := goose.GetDBVersion(m.db.DB)

	if err := goose.UpContext(context.Background(), m.db.DB, "."); err != nil {
		return err
	}

	after, _ := goose.GetDBVersion(m.db.DB)

	if after > before {
		m.logger.Info("migrations: applied", logKeyFrom, before, logKeyTo, after)
	} else {
		m.logger.Info("migrations: up to date", logKeyVersion, after)
	}

	return nil
}
