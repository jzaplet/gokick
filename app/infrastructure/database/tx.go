// Package database holds the driver-neutral pieces every SQL adapter shares: the
// transaction-in-context carrier and the migrator port. Driver-specific code (the
// connection manager, DSN tuning, the migration set) lives in the adapter package
// (infrastructure/sqlite), so importing this package never links a driver.
package database

import (
	"context"

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
