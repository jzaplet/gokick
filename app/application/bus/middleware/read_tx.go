package middleware

import (
	"context"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// ReadTxMiddleware runs a query inside a read-only transaction
// (shared.Transactor.BeginReadTx). It closes the QueryBus chain, after
// TenantMiddleware, because the transaction is what carries the tenant scope on
// Postgres: row-level security reads a transaction-local setting, so a tenant-plane
// read outside a transaction would see no row at all. On SQLite BeginReadTx is a
// no-op and queries read straight from the pool, as before.
func ReadTxMiddleware(tx shared.Transactor) bus.Middleware {
	return func(ctx context.Context, name string, q any, next func(ctx context.Context) (any, error)) (any, error) {
		txCtx, end, err := tx.BeginReadTx(ctx)
		if err != nil {
			return nil, err
		}
		defer end()
		return next(txCtx)
	}
}
