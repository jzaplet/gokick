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

	db, err := sqlx.Open("sqlite3", config.DBPath)
	if err != nil {
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
	if _, err := db.Exec("PRAGMA journal_mode=" + journalMode); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
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
