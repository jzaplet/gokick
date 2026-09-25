// Package postgres is the Postgres adapter. Its Manager holds two connection pools,
// one per database role, and implements shared.Transactor: a transaction opens on
// the pool of the plane in ctx (shared.Plane), and on the tenant plane it first
// scopes itself to the active tenant, which row-level security then enforces. The
// repositories live in the sub-packages (user, tenant, token, run, audit) and embed
// BaseRepository, which resolves where each statement runs.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// DriverName is the database/sql driver the pools use (pgx's stdlib adapter); sqlx
// derives the $n placeholder style from it.
const DriverName = "pgx"

// tenantSetting is the transaction-local setting the row-level security policies
// read (gokick_current_tenant() in migrations/postgres).
const tenantSetting = "app.tenant_id"

// Manager owns the two role pools and implements shared.Transactor.
//
//   - app (APP_DB_URL, gokick_app): the tenant plane. The role is bound by the
//     tenant policies, so everything it reads or writes stays inside the tenant
//     the transaction was scoped to.
//   - system (APP_DB_SYSTEM_URL, gokick_system): the platform and system planes,
//     and the cross-tenant plumbing (login, audit, worker claims). The role
//     bypasses row-level security.
type Manager struct {
	app         *sqlx.DB
	system      *sqlx.DB
	multitenant bool
}

// NewManager opens both pools. Like database/sql it connects lazily: an
// unreachable server surfaces on first use (at startup, the migrations), not here.
func NewManager(cfg *config.Config) (*Manager, error) {
	app, err := openPool(cfg.DBURL, "APP_DB_URL", cfg)
	if err != nil {
		return nil, err
	}
	system, err := openPool(cfg.DBSystemURL, "APP_DB_SYSTEM_URL", cfg)
	if err != nil {
		_ = app.Close()
		return nil, err
	}
	return &Manager{app: app, system: system, multitenant: cfg.Multitenancy}, nil
}

// openPool opens one role's pool with the session settings every connection
// starts with: UTC, the application name (visible in pg_stat_activity) and the
// configured lock, statement and idle-transaction limits.
func openPool(dsn, key string, cfg *config.Config) (*sqlx.DB, error) {
	pc, err := connConfig(dsn, key, "gokick")
	if err != nil {
		return nil, err
	}
	pc.RuntimeParams["lock_timeout"] = milliseconds(cfg.DBLockTimeout)
	pc.RuntimeParams["statement_timeout"] = milliseconds(cfg.DBStatementTimeout)
	pc.RuntimeParams["idle_in_transaction_session_timeout"] = milliseconds(cfg.DBIdleTxTimeout)

	db := sqlx.NewDb(stdlib.OpenDB(*pc, stdlib.OptionAfterConnect(scanTimesInUTC)), DriverName)
	maxConns := database.PoolSize(cfg.DBMaxConns, dbMaxConnsFloor, dbMaxConnsCeil)
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	// Recycle connections now and then, so a pool rebalances after a failover
	// and no server backend lives forever.
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}

// scanTimesInUTC makes the connection read timestamptz values back as UTC times —
// the same instant pgx would return in the process's local zone, but in the zone
// the SQLite adapter returns and the application writes, so a time reads back
// equal (==, not just Equal) and serializes the same on both backends.
func scanTimesInUTC(_ context.Context, conn *pgx.Conn) error {
	conn.TypeMap().RegisterType(&pgtype.Type{
		Name:  "timestamptz",
		OID:   pgtype.TimestamptzOID,
		Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
	})
	return nil
}

// connConfig parses dsn (the value of the variable key) with the settings every
// connection of the adapter starts with: UTC, and appName as the application name
// visible in pg_stat_activity unless the DSN names one.
func connConfig(dsn, key, appName string) (*pgx.ConnConfig, error) {
	pc, err := pgx.ParseConfig(dsn)
	if err != nil {
		// The parse error may quote the DSN; name the variable instead.
		return nil, fmt.Errorf("postgres: invalid %s", key)
	}
	pc.RuntimeParams["TimeZone"] = "UTC"
	if pc.RuntimeParams["application_name"] == "" {
		pc.RuntimeParams["application_name"] = appName
	}
	return pc, nil
}

// milliseconds renders d as a Postgres time setting (integer milliseconds; 0
// disables the limit). A positive d rounds UP: a sub-millisecond limit must not
// truncate to 0 and silently switch the limit off.
func milliseconds(d time.Duration) string {
	return strconv.FormatInt(int64((d+time.Millisecond-1)/time.Millisecond), 10)
}

// dbMaxConnsFloor / dbMaxConnsCeil bound the auto cap of EACH pool
// (database.PoolSize: 2×NumCPU when APP_DB_MAX_CONNS is unset). The ceiling is
// lower than SQLite's: two pools per process, times the replicas, must stay well
// under the server's max_connections (100 by default).
const (
	dbMaxConnsFloor = 4
	dbMaxConnsCeil  = 16
)

// App is the tenant-plane pool (gokick_app).
func (m *Manager) App() *sqlx.DB { return m.app }

// System is the system-plane pool (gokick_system, BYPASSRLS).
func (m *Manager) System() *sqlx.DB { return m.system }

// Multitenant reports the configured enforcement mode (APP_MULTITENANCY).
func (m *Manager) Multitenant() bool { return m.multitenant }

// Close closes both pools.
func (m *Manager) Close() error {
	return errors.Join(m.app.Close(), m.system.Close())
}

// errTxForbidden mirrors the SQLite adapter: a durable run handler must not open a
// transaction implicitly (see shared.ContextForbidTx). On Postgres a long
// transaction does not freeze the database, but it still holds its row locks and a
// connection for the handler's whole lifetime and blocks vacuum.
var errTxForbidden = errors.New(
	"postgres: BeginTx called in a no-transaction zone — a durable run handler must not " +
		"open a transaction (it would hold row locks and a connection for the run's " +
		"lifetime); persist state via the Checkpointer or enqueue a command/run")

// txScope is what a transaction the Manager opens is bound to: the role (the
// system pool for the cross-tenant planes) and, on the tenant plane, the tenant.
type txScope struct {
	crossTenant bool
	tenantID    string
}

type txScopeKey struct{}

// scope resolves ctx's plane and tenant into a txScope. A tenant plane without a
// tenant fails closed under multitenancy (shared.RequireTenant) and scopes to the
// default tenant otherwise.
func (m *Manager) scope(ctx context.Context) (txScope, error) {
	if shared.PlaneFromContext(ctx).CrossTenant() {
		return txScope{crossTenant: true}, nil
	}
	tenantID, err := shared.RequireTenant(
		shared.TenantIDFromContext(ctx), shared.Multitenancy(m.multitenant))
	if err != nil {
		return txScope{}, err
	}
	return txScope{tenantID: tenantID}, nil
}

// BeginTx opens a read-write transaction on the plane in ctx.
func (m *Manager) BeginTx(ctx context.Context) (context.Context, error) {
	if shared.IsTxForbidden(ctx) {
		return ctx, errTxForbidden
	}
	s, err := m.scope(ctx)
	if err != nil {
		return ctx, err
	}
	tx, err := m.begin(ctx, s, nil)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(database.ContextWithTx(ctx, tx), txScopeKey{}, s), nil
}

// BeginReadTx opens a READ ONLY transaction for one query, on the plane in ctx.
// It is what lets a tenant-plane read see its tenant's rows at all: the tenant
// scope is transaction-local. The returned end rolls the transaction back — it
// wrote nothing, so a rollback ends it as well as a commit would, and cannot fail
// on a write.
//
// When ctx already carries a transaction the query joins it — but only one of the
// same scope: joined, a platform query would see one tenant's rows, and a tenant
// query inside a system transaction would run without row-level security. Any
// other scope is refused.
//
// It does not honor the no-transaction zone: that rule is about long work holding
// a transaction open, and a read transaction lives exactly as long as one query.
func (m *Manager) BeginReadTx(ctx context.Context) (context.Context, func(), error) {
	s, err := m.scope(ctx)
	if err != nil {
		return ctx, nil, err
	}
	if database.TxFromContext(ctx) != nil {
		if open, _ := ctx.Value(txScopeKey{}).(txScope); open != s {
			return ctx, nil, errors.New("postgres: a query cannot join the open " +
				"transaction — it runs on another plane or tenant")
		}
		return ctx, func() {}, nil
	}
	tx, err := m.begin(ctx, s, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ctx, nil, err
	}
	txCtx := context.WithValue(database.ContextWithTx(ctx, tx), txScopeKey{}, s)
	return txCtx, func() { _ = tx.Rollback() }, nil
}

// begin opens a transaction on the pool of scope s. On the tenant plane it scopes
// the transaction to the tenant before handing it out.
func (m *Manager) begin(ctx context.Context, s txScope, opts *sql.TxOptions) (*sqlx.Tx, error) {
	if s.crossTenant {
		return m.system.BeginTxx(ctx, opts)
	}
	tx, err := m.app.BeginTxx(ctx, opts)
	if err != nil {
		return nil, err
	}
	// is_local = true: the setting ends with the transaction, so the pooled
	// connection never carries this tenant into its next transaction.
	if _, err := tx.ExecContext(ctx,
		`SELECT set_config('`+tenantSetting+`', $1, true)`, s.tenantID); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("postgres: scope the transaction to its tenant: %w", err)
	}
	return tx, nil
}

func (m *Manager) Commit(ctx context.Context) error { return database.CommitTx(ctx) }

func (m *Manager) Rollback(ctx context.Context) error { return database.RollbackTx(ctx) }
