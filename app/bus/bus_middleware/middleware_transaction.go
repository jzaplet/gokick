package bus_middleware

import (
	"context"
	"myapp/app/bus"
	"myapp/app/database"
)

func TransactionMiddleware(db *database.SqliteManager) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		tx, err := db.BeginTx(ctx)
		if err != nil {
			return nil, err
		}

		ctxWithTx := database.ContextWithTx(ctx, tx)
		result, err := next(ctxWithTx)

		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, commitErr
		}
		return result, nil
	}
}
