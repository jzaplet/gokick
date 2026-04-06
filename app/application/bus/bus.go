package bus

import "context"

type Middleware func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error)

type Bus struct {
	middlewares []Middleware
}

func newBus(middlewares ...Middleware) *Bus {
	return &Bus{middlewares: middlewares}
}

func (b *Bus) execute(
	ctx context.Context,
	name string,
	cmd any,
	fn func(ctx context.Context) (any, error),
) (any, error) {
	chain := fn
	for i := len(b.middlewares) - 1; i >= 0; i-- {
		mw := b.middlewares[i]
		next := chain
		chain = func(ctx context.Context) (any, error) {
			return mw(ctx, name, cmd, func(ctx context.Context) (any, error) {
				return next(ctx)
			})
		}
	}
	return chain(ctx)
}
