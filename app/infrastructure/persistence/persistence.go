// Package persistence opens the database adapter and hands the rest of the
// application its ports. It is the ONE place that knows which adapter backs the
// repositories: Wire takes every port from the Store (wire.FieldsOf), so neither
// DI nor any handler names a concrete repository type. Adding the Postgres
// adapter means branching here on the configured driver — nothing else changes.
package persistence

import (
	"log/slog"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	sqliterun "gokick/app/infrastructure/sqlite/run"
	sqlitetenant "gokick/app/infrastructure/sqlite/tenant"
	sqlitetoken "gokick/app/infrastructure/sqlite/token"
	sqliteuser "gokick/app/infrastructure/sqlite/user"
)

// logMsgCloseFailed is logged when closing the pool at shutdown fails — the
// process is exiting anyway, so it is reported, not returned.
const logMsgCloseFailed = "persistence: closing the database failed"

// Store is everything the application needs from the database, as ports. The
// platform ports are the same concrete repositories as their tenant-scoped
// counterparts, exposed separately so a handler asks for exactly the plane it
// works on (see /gk-multitenancy).
type Store struct {
	Users           user.Repository
	PlatformUsers   user.PlatformRepository
	Tokens          token.Repository
	Runs            run.Repository
	Tenants         tenant.Repository
	PlatformTenants tenant.PlatformRepository
	Audit           shared.AuditLogger
	Tx              shared.Transactor
	Migrator        database.Migrator
}

// Open connects the database and builds the Store. The returned cleanup closes
// the connection pool; Wire runs it when the application shuts down.
func Open(cfg *config.Config, logger *slog.Logger) (*Store, func(), error) {
	mgr, err := sqlite.NewManager(cfg)
	if err != nil {
		return nil, nil, err
	}

	users := sqliteuser.NewRepository(mgr)
	tenants := sqlitetenant.NewRepository(mgr)
	store := &Store{
		Users:           users,
		PlatformUsers:   users,
		Tokens:          sqlitetoken.NewRepository(mgr),
		Runs:            sqliterun.NewRepository(mgr),
		Tenants:         tenants,
		PlatformTenants: tenants,
		Audit:           sqliteaudit.NewRepository(mgr),
		Tx:              mgr,
		Migrator:        sqlite.NewMigrator(mgr, logger),
	}
	cleanup := func() {
		if err := mgr.Close(); err != nil {
			logger.Warn(logMsgCloseFailed, shared.LogKeyError, err)
		}
	}
	return store, cleanup, nil
}
