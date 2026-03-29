package database

import (
	"context"
	"myapp/app/env"
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

func NewSqliteManager(config *env.Config) (*SqliteManager, error) {
	dir := filepath.Dir(config.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db, err := sqlx.Open("sqlite3", config.DBPath)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
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

func (m *SqliteManager) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return m.db.BeginTxx(ctx, nil)
}

func ContextWithTx(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

func TxFromContext(ctx context.Context) *sqlx.Tx {
	tx, _ := ctx.Value(txKey).(*sqlx.Tx)
	return tx
}
