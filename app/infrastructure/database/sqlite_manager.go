package database

import (
	"context"
	"fmt"
	"gokick/app/infrastructure/config"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "github.com/ncruces/go-sqlite3/driver"
)

type txKeyType struct{}

var txKey = txKeyType{}

type SqliteManager struct {
	db *sqlx.DB
}

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
	// foreign_keys is per-connection in SQLite — setting it via DSN
	// guarantees every pooled connection has it on, not just the one the
	// first PRAGMA executed against.
	//
	// The file: prefix is required for ncruces to parse the query string
	// at all (driver.go newConnector: query parsing is gated on file:).
	dsn := "file:" + config.DBPath +
		"?_txlock=immediate" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(on)"

	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// journal_mode is filesystem-persistent (WAL flips the on-disk header)
	// so a one-shot exec on the pool is enough; later connections inherit
	// it from the file.
	if _, err := db.Exec("PRAGMA journal_mode=" + journalMode); err != nil {
		return nil, err
	}

	return &SqliteManager{db: db}, nil
}

func (m *SqliteManager) DB() *sqlx.DB {
	return m.db
}

func (m *SqliteManager) Close() error {
	return m.db.Close()
}

func (m *SqliteManager) BeginTx(ctx context.Context) (context.Context, error) {
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
