package persistence

import (
	"log/slog"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/postgres"
	pgaudit "gokick/app/infrastructure/postgres/audit"
	pgrun "gokick/app/infrastructure/postgres/run"
	pgtenant "gokick/app/infrastructure/postgres/tenant"
	pgtoken "gokick/app/infrastructure/postgres/token"
	pguser "gokick/app/infrastructure/postgres/user"
)

// openPostgres opens the two role pools and returns the Store with their Close.
func openPostgres(cfg *config.Config, logger *slog.Logger) (*Store, func() error, error) {
	mgr, err := postgres.NewManager(cfg)
	if err != nil {
		return nil, nil, err
	}
	return PostgresStore(mgr, cfg.DBMigrateURL, logger), mgr.Close, nil
}

// PostgresStore builds the Store over an already open Postgres manager, migrating
// as the owner behind migrateURL. Open uses it, and so does the test harness
// (internal/testfx), which keeps the manager for fixture writes — the twin of
// SQLiteStore.
func PostgresStore(mgr *postgres.Manager, migrateURL string, logger *slog.Logger) *Store {
	users := pguser.NewRepository(mgr)
	tenants := pgtenant.NewRepository(mgr)
	return &Store{
		Users:           users,
		PlatformUsers:   users,
		Tokens:          pgtoken.NewRepository(mgr),
		Runs:            pgrun.NewRepository(mgr),
		Tenants:         tenants,
		PlatformTenants: tenants,
		Audit:           pgaudit.NewRepository(mgr),
		Tx:              mgr,
		Locker:          postgres.NewLocker(mgr),
		Migrator:        postgres.NewMigrator(mgr, migrateURL, logger),
	}
}
