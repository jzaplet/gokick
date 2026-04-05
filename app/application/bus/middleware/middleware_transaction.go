package middleware

import (
	"context"
	"myapp/app/application/bus"
	"myapp/app/domain/shared"
)

func TransactionMiddleware(tx shared.Transactor) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		ctxWithTx, err := tx.BeginTx(ctx)
		if err != nil {
			return nil, err
		}

		result, err := next(ctxWithTx)

		if err != nil {
			_ = tx.Rollback(ctxWithTx)
			return nil, err
		}
		if commitErr := tx.Commit(ctxWithTx); commitErr != nil {
			return nil, commitErr
		}
		return result, nil
	}
}
