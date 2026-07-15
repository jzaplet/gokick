package bus

import "context"

// Dispatch runs a write command through the CommandBus middleware chain and
// returns the handler's typed result. It takes *CommandBus — not the bare inner
// *Bus — so the bus↔operation pairing is compile-checked: a write command can no
// longer be handed to the QueryBus (which runs with no Transaction/Audit/
// DispatchEvents), because Query wants a *QueryBus and Dispatch a *CommandBus.
func Dispatch[R any](
	ctx context.Context,
	b *CommandBus,
	name string,
	cmd any,
	fn func(ctx context.Context) (R, error),
) (R, error) {
	return exec(ctx, b.inner, name, cmd, fn)
}

// DispatchVoid is Dispatch for a command that returns no value.
func DispatchVoid(
	ctx context.Context,
	b *CommandBus,
	name string,
	cmd any,
	fn func(ctx context.Context) error,
) error {
	return execVoid(ctx, b.inner, name, cmd, fn)
}

// Query runs a read through the QueryBus chain (Recovery→Logging→Authorize→Tenant)
// and returns the handler's typed result. Taking *QueryBus is the read-side half
// of the compile-checked pairing (see Dispatch).
func Query[R any](
	ctx context.Context,
	b *QueryBus,
	name string,
	q any,
	fn func(ctx context.Context) (R, error),
) (R, error) {
	return exec(ctx, b.inner, name, q, fn)
}

// SystemDispatch runs an operator-trusted CLI command through the SystemCommandBus
// and returns the handler's typed result.
func SystemDispatch[R any](
	ctx context.Context,
	b *SystemCommandBus,
	name string,
	cmd any,
	fn func(ctx context.Context) (R, error),
) (R, error) {
	return exec(ctx, b.inner, name, cmd, fn)
}

// SystemDispatchVoid is SystemDispatch for a command that returns no value.
func SystemDispatchVoid(
	ctx context.Context,
	b *SystemCommandBus,
	name string,
	cmd any,
	fn func(ctx context.Context) error,
) error {
	return execVoid(ctx, b.inner, name, cmd, fn)
}
