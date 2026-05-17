package middleware

import (
	"context"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// JobDispatcherMiddleware injects the dispatcher into ctx so command/event
// handlers can call shared.JobDispatcherFromContext(ctx).Enqueue(...).
//
// It must sit OUTSIDE TransactionMiddleware in the chain so the dispatcher
// is available before the transaction begins, but the Enqueue call itself
// uses Conn(ctx) — when invoked inside a handler running under Transaction,
// the INSERT joins that transaction (atomic business write + job enqueue).
func JobDispatcherMiddleware(dispatcher shared.JobDispatcher) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		return next(shared.ContextWithJobDispatcher(ctx, dispatcher))
	}
}
