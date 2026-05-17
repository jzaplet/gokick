package middleware

import (
	"context"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// SkipsTransaction is the opt-out marker for commands that MUST run
// outside a bus-managed transaction. Required for handlers that touch
// raw-pool repositories (e.g. user.RecordFailedLogin / ResetFailedLogin)
// while inside their own command — wrapping such a handler in tx
// self-deadlocks under SQLite, because the raw-pool write blocks
// waiting for the very tx the handler hasn't returned from yet.
//
// Use sparingly. The commands that need it are the ones where
// "consistency across multiple writes" isn't actually buying anything
// (Login: a failed token Save just returns an error to the user).
type SkipsTransaction interface {
	SkipTransaction()
}

func TransactionMiddleware(tx shared.Transactor) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		if _, skip := cmd.(SkipsTransaction); skip {
			return next(ctx)
		}

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
