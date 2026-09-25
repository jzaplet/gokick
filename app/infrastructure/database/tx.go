// Package database holds the driver-neutral pieces every SQL adapter shares: the
// Driver name, the transaction-in-context carrier (and ending it), the pool-size
// rule and the migrator port. Driver-specific code (the connection manager, DSN
// tuning, the migration set) lives in the adapter packages (infrastructure/sqlite,
// infrastructure/postgres), so importing this package never links a driver.
package database

import (
	"context"
	"errors"
	"runtime"

	"github.com/jmoiron/sqlx"
)

type txKeyType struct{}

var txKey = txKeyType{}

// ContextWithTx stores the open transaction in ctx. An adapter's BeginTx calls it;
// its repositories resolve the transaction back through TxFromContext, so a bus
// command's repos join the transaction without it being passed around.
func ContextWithTx(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

// TxFromContext returns the transaction stored by ContextWithTx, or nil when ctx
// carries none (the repository then uses the connection pool).
func TxFromContext(ctx context.Context) *sqlx.Tx {
	tx, _ := ctx.Value(txKey).(*sqlx.Tx)
	return tx
}

var errNoTx = errors.New("database: no transaction in context")

// CommitTx / RollbackTx end the transaction ContextWithTx stored in ctx — the
// Commit and Rollback of every adapter's shared.Transactor.
func CommitTx(ctx context.Context) error {
	tx := TxFromContext(ctx)
	if tx == nil {
		return errNoTx
	}
	return tx.Commit()
}

func RollbackTx(ctx context.Context) error {
	tx := TxFromContext(ctx)
	if tx == nil {
		return errNoTx
	}
	return tx.Rollback()
}

// PoolSize resolves an adapter's connection-pool cap: the configured value
// (APP_DB_MAX_CONNS), or — when that is unset (<= 0) — 2×NumCPU clamped to the
// adapter's [floor, ceil].
func PoolSize(configured, floor, ceil int) int {
	if configured > 0 {
		return configured
	}
	return min(max(2*runtime.NumCPU(), floor), ceil)
}
