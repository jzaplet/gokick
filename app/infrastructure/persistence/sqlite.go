//go:build !nosqlite

package persistence

import (
	"log/slog"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/sqlite"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	sqliterun "gokick/app/infrastructure/sqlite/run"
	sqlitetenant "gokick/app/infrastructure/sqlite/tenant"
	sqlitetoken "gokick/app/infrastructure/sqlite/token"
	sqliteuser "gokick/app/infrastructure/sqlite/user"
)

// openSQLite opens the SQLite pool and returns the Store with the pool's Close.
func openSQLite(cfg *config.Config, logger *slog.Logger) (*Store, func() error, error) {
	mgr, err := sqlite.NewManager(cfg)
	if err != nil {
		return nil, nil, err
	}
	return SQLiteStore(mgr, logger), mgr.Close, nil
}

// SQLiteStore builds the Store over an already open SQLite manager. Open uses it,
// and so does the test harness (internal/testfx), which keeps the manager for
// fixture writes that reach past the repositories — so both get the very same
// wiring.
func SQLiteStore(mgr *sqlite.Manager, logger *slog.Logger) *Store {
	users := sqliteuser.NewRepository(mgr)
	tenants := sqlitetenant.NewRepository(mgr)
	return &Store{
		Users:           users,
		PlatformUsers:   users,
		Tokens:          sqlitetoken.NewRepository(mgr),
		Runs:            sqliterun.NewRepository(mgr),
		Tenants:         tenants,
		PlatformTenants: tenants,
		Audit:           sqliteaudit.NewRepository(mgr),
		Tx:              mgr,
		Locker:          sqlite.Locker{},
		Migrator:        sqlite.NewMigrator(mgr, logger),
	}
}
