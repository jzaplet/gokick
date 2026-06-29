package middleware

import (
	"context"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// RunDispatcherMiddleware injects the durable-run dispatcher into ctx so command/
// event handlers can call shared.RunDispatcherFromContext(ctx).Enqueue(...).
//
// It sits OUTSIDE TransactionMiddleware so the
// dispatcher is available before the transaction begins; the Enqueue call uses
// Conn(ctx), so inside a handler running under Transaction the INSERT joins that
// transaction (atomic business write + run enqueue).
func RunDispatcherMiddleware(dispatcher shared.RunDispatcher) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		return next(shared.ContextWithRunDispatcher(ctx, dispatcher))
	}
}
