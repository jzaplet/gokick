package bus

import "context"

// exec is the package-internal generic core: it runs fn through b's middleware
// chain and type-asserts the result. It takes the bare *Bus and is unexported —
// call sites reach it through the wrapper-typed Dispatch/Query/SystemDispatch
// (dispatch.go), which is what makes the bus↔operation pairing compile-checked.
func exec[R any](
	ctx context.Context,
	b *Bus,
	name string,
	cmd any,
	fn func(ctx context.Context) (R, error),
) (R, error) {
	var zero R

	result, err := b.execute(ctx, name, cmd, func(ctx context.Context) (any, error) {
		return fn(ctx)
	})
	if err != nil {
		return zero, err
	}
	if result == nil {
		return zero, nil
	}
	return result.(R), nil
}
