package database

import (
	"context"
	"gokick/migrations"
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
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
		m.logger.Info("migrations: applied", "from", before, "to", after)
	} else {
		m.logger.Info("migrations: up to date", "version", after)
	}

	return nil
}
