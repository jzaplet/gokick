package bus

import "context"

func ExecVoid(
	ctx context.Context,
	b *Bus,
	name string,
	cmd any,
	fn func(ctx context.Context) error,
) error {
	_, err := Exec[any](ctx, b, name, cmd, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	})
	return err
}
