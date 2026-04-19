package bus

import "context"

func Exec[R any](
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
