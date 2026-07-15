package database

import (
	"context"
	"fmt"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jmoiron/sqlx"
	_ "github.com/ncruces/go-sqlite3/driver"
)

type txKeyType struct{}

var txKey = txKeyType{}

type SqliteManager struct {
	db          *sqlx.DB
	multitenant bool
}

// Multitenant reports the configured enforcement mode (APP_MULTITENANCY). When
// true, BaseRepository.Tenant fails closed (panics) on a missing tenant instead
// of falling back to the default tenant.
func (m *SqliteManager) Multitenant() bool { return m.multitenant }

func NewSqliteManager(config *config.Config) (*SqliteManager, error) {
	dir := filepath.Dir(config.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	journalMode := config.DBJournalMode
	if journalMode == "" {
		journalMode = "WAL"
	}
	// PRAGMA values can't be parameterised, so guard against arbitrary
	// SQL injection from misconfigured env vars by whitelisting the
	// modes that this app actually supports. WAL is the default; DELETE
	// is needed for bind-mounted dev DBs; MEMORY exists for tests.
	switch journalMode {
	case "WAL", "DELETE", "MEMORY":
	default:
		return nil, fmt.Errorf(
			"database: APP_DB_JOURNAL_MODE must be WAL|DELETE|MEMORY, got %q", journalMode)
	}

	// _txlock=immediate makes sql.DB issue BEGIN IMMEDIATE for every
	// transaction. Required because the bus pattern is read-then-CPU-then-
	// write inside one tx (e.g. CreateUser: FindByNickname → bcrypt → Save),
	// and under WAL with the default DEFERRED a concurrent commit during
	// the CPU window invalidates the read snapshot and the follow-up write
	// fails immediately as SQLITE_BUSY_SNAPSHOT (which busy_timeout cannot
	// retry — it's a fatal-to-tx outcome). IMMEDIATE takes the write lock
	// at BEGIN so the snapshot can't go stale mid-handler.
	//
	// busy_timeout(5000) pairs with the above: short overlaps between the
	// scheduler/worker poll writes and a user-driven command serialize via
	// a 5s wait instead of bubbling "database is locked" to the caller.
	//
	// foreign_keys AND journal_mode are both per-connection in SQLite — setting
	// them via DSN _pragma guarantees EVERY pooled connection gets them, not just
	// the one a first one-shot PRAGMA happened to run against. (WAL/DELETE also
	// flip the on-disk header so they'd persist regardless, but MEMORY is purely
	// per-connection, so a one-shot exec applied it to a single pooled connection
	// only — the bug this closes.)
	//
	// The file: prefix is required for ncruces to parse the query string
	// at all (driver.go newConnector: query parsing is gated on file:).
	dsn := "file:" + config.DBPath +
		"?_txlock=immediate" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(on)" +
		"&_pragma=journal_mode(" + journalMode + ")"

	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Bound the connection pool. SQLite serializes writes on a single writer
	// (_txlock=immediate), so a bigger pool does NOT raise write throughput — its
	// job is to cap MEMORY (each ncruces WASM connection carries a WASM instance +
	// page cache, a few MB) and to apply BACKPRESSURE: a blocked writer waits
	// cheaply at Go's pool gate instead of every burst goroutine holding a live
	// WASM connection (unbounded → reachable OOM, F-047). Reads run concurrently
	// under WAL, so the cap tracks read concurrency + a little write headroom.
	maxConns := config.DBMaxConns
	if maxConns <= 0 {
		maxConns = autoMaxConns()
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	return &SqliteManager{db: db, multitenant: config.Multitenancy}, nil
}

// dbMaxConnsFloor / dbMaxConnsCeil bound the auto pool cap. The floor keeps a
// tiny box (a 2-vCPU droplet) usable; the ceiling bounds WASM-connection memory
// on a big-core host, where extra connections past read concurrency buy nothing
// (writes serialize regardless).
const (
	dbMaxConnsFloor = 4
	dbMaxConnsCeil  = 32
)

// autoMaxConns derives the pool cap from the CPU count when APP_DB_MAX_CONNS is
// unset. 2×NumCPU covers concurrent readers plus a little write-queue headroom;
// clamped to [floor, ceil]. Cloud RAM scales with vCPU, so scaling by CPU
// implicitly tracks the memory budget (2 vCPU / 2 GB → 4; 10-core → 20; 16-core
// → capped 32). Override with APP_DB_MAX_CONNS for an unusual RAM:CPU ratio.
func autoMaxConns() int {
	n := 2 * runtime.NumCPU()
	if n < dbMaxConnsFloor {
		return dbMaxConnsFloor
	}
	if n > dbMaxConnsCeil {
		return dbMaxConnsCeil
	}
	return n
}

func (m *SqliteManager) DB() *sqlx.DB {
	return m.db
}

func (m *SqliteManager) Close() error {
	return m.db.Close()
}

func (m *SqliteManager) BeginTx(ctx context.Context) (context.Context, error) {
	// Fail closed in a no-transaction zone (a durable run handler): a long handler
	// inside a transaction would hold the global SQLite write lock for its whole
	// lifetime and freeze every other write. See shared.ContextForbidTx.
	if shared.IsTxForbidden(ctx) {
		return ctx, fmt.Errorf(
			"database: BeginTx called in a no-transaction zone — a durable run handler " +
				"must not open a transaction (it would hold the global write lock for the run's " +
				"lifetime); persist state via the Checkpointer or enqueue a command/run",
		)
	}
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, txKey, tx), nil
}

func (m *SqliteManager) Commit(ctx context.Context) error {
	tx := TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("database: no transaction in context")
	}
	return tx.Commit()
}

func (m *SqliteManager) Rollback(ctx context.Context) error {
	tx := TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("database: no transaction in context")
	}
	return tx.Rollback()
}

func TxFromContext(ctx context.Context) *sqlx.Tx {
	tx, _ := ctx.Value(txKey).(*sqlx.Tx)
	return tx
}
