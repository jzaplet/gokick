// Package persistence opens the database adapter and hands the rest of the
// application its ports. It is the ONE place that knows which adapter backs the
// repositories: Wire takes every port from the Store (wire.FieldsOf), so neither
// DI nor any handler names a concrete repository type. Open branches on the
// configured driver (APP_DB_DRIVER); each adapter's opener lives in its own
// build-tagged file, so a binary built without an adapter (-tags nosqlite) does
// not link it at all and refuses that driver at startup.
package persistence

import (
	"errors"
	"fmt"
	"log/slog"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
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

// Open connects the configured adapter and builds the Store. The returned
// cleanup closes the connection pool; Wire runs it when the application shuts
// down.
func Open(cfg *config.Config, logger *slog.Logger) (*Store, func(), error) {
	var (
		store   *Store
		closeFn func() error
		err     error
	)
	switch cfg.DBDriver {
	case database.DriverSQLite:
		store, closeFn, err = openSQLite(cfg, logger)
	case database.DriverPostgres:
		err = errPostgresIncomplete
	default:
		err = notBuilt(cfg.DBDriver)
	}
	if err != nil {
		return nil, nil, err
	}
	return store, closer(closeFn, logger), nil
}

// errPostgresIncomplete refuses Postgres until its adapter is whole: the
// connection manager, the schema and its row-level security exist
// (infrastructure/postgres, migrations/postgres), the repositories do not yet —
// phase 4 of the Postgres adapter plan — so no Store can be built on it.
var errPostgresIncomplete = errors.New("persistence: database driver \"postgres\" is not " +
	"available yet — the Postgres adapter has no repositories so far (see " +
	"docs/framework/postgres-adapter-plan.md); use APP_DB_DRIVER=sqlite")

// notBuilt is the error for a driver this binary has no adapter for (excluded by
// a build tag).
func notBuilt(d database.Driver) error {
	return fmt.Errorf("persistence: database driver %q is not built into this binary", d)
}

// closer returns the Store cleanup: close the pool, and log (not return) a
// failure — the process is on its way out.
func closer(closeFn func() error, logger *slog.Logger) func() {
	return func() {
		if err := closeFn(); err != nil {
			logger.Warn(logMsgCloseFailed, shared.LogKeyError, err)
		}
	}
}
