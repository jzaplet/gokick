package postgres

import (
	"context"
	"database/sql"
	"log/slog"

	"gokick/app/infrastructure/database"
	"gokick/migrations"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Migrator applies the embedded Postgres migration set (migrations.Postgres) as the
// schema owner (APP_DB_MIGRATE_URL) — the adapter's database.Migrator. The owner's
// connection lives only for the run: the application never keeps a pool of the
// role that could switch row security off.
type Migrator struct {
	mgr        *Manager
	migrateURL string
	logger     *slog.Logger
}

func NewMigrator(mgr *Manager, migrateURL string, logger *slog.Logger) *Migrator {
	return &Migrator{mgr: mgr, migrateURL: migrateURL, logger: logger}
}

// migrationLockPollSeconds / migrationLockAttempts: a replica waiting for another
// one's migrations retries the lock every second, for up to five minutes. (goose's
// default polls every 5 s, which makes every waiting replica start seconds late.)
const (
	migrationLockPollSeconds = 1
	migrationLockAttempts    = 300
)

// NewMigrationProvider builds the goose provider for the Postgres migration set
// over db, which must connect as the schema owner. Its session locker takes a
// Postgres advisory lock for the whole run, so replicas starting together migrate
// one at a time instead of racing the same DDL. As with SQLite, the global Go
// migration registry is disabled: the project ships SQL migrations only.
func NewMigrationProvider(db *sql.DB) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker(
		lock.WithLockTimeout(migrationLockPollSeconds, migrationLockAttempts))
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, migrations.Postgres,
		goose.WithDisableGlobalRegistry(true),
		goose.WithLogger(goose.NopLogger()),
		goose.WithSessionLocker(locker),
	)
}

// OpenOwner opens a short-lived connection pool as the schema owner — for the
// migrations only. No statement or lock limit applies: a migration may
// legitimately run long.
func OpenOwner(migrateURL string) (*sql.DB, error) {
	pc, err := connConfig(migrateURL, "APP_DB_MIGRATE_URL", "gokick-migrate")
	if err != nil {
		return nil, err
	}
	return stdlib.OpenDB(*pc), nil
}

// RunUp migrates to the latest version, then verifies the runtime roles
// (Manager.VerifyRoles) — the startup gate that refuses to serve a single request
// over a role that would bypass the tenant wall. Application.Run calls it before
// every command.
func (m *Migrator) RunUp() error {
	ctx := context.Background()
	db, err := OpenOwner(m.migrateURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	provider, err := NewMigrationProvider(db)
	if err != nil {
		return err
	}
	if err := database.MigrateUp(ctx, provider, m.logger); err != nil {
		return err
	}
	return m.mgr.VerifyRoles(ctx)
}
