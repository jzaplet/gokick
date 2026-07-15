package shared

import (
	"context"
	"time"
)

// RunDispatcher enqueues a durable run from inside a command/event handler — the
// long-running primitive. The run's HANDLER executes
// outside a transaction, but Enqueue itself honors Conn(ctx): called from a
// CommandBus handler (inside a tx) the INSERT joins that transaction, so the
// business write and the run enqueue commit atomically.
//
// maxRetries is a required positional parameter (a per-kind decision, not a
// default): 0 = run once, higher for flaky work. It caps LOGIC retries only —
// crash reclaims are bounded separately by the worker.
type RunDispatcher interface {
	Enqueue(
		ctx context.Context,
		kind string,
		maxRetries int,
		payload any,
		opts ...EnqueueOption,
	) error
}

// EnqueueOptions / WithDelay tune an enqueue. Delay 0 = run as soon as possible.
type EnqueueOptions struct {
	Delay time.Duration
}

type EnqueueOption func(*EnqueueOptions)

func WithDelay(d time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) { o.Delay = d }
}

type runDispatcherKey struct{}

func ContextWithRunDispatcher(ctx context.Context, d RunDispatcher) context.Context {
	return context.WithValue(ctx, runDispatcherKey{}, d)
}

// RunDispatcherFromContext returns the dispatcher injected by the bus middleware.
// Outside the bus (CLI, tests) it returns a no-op dispatcher so handlers never
// nil-check; enqueue calls are silently dropped.
func RunDispatcherFromContext(ctx context.Context) RunDispatcher {
	if d, ok := ctx.Value(runDispatcherKey{}).(RunDispatcher); ok {
		return d
	}
	return noopRunDispatcher{}
}

type noopRunDispatcher struct{}

func (noopRunDispatcher) Enqueue(context.Context, string, int, any, ...EnqueueOption) error {
	return nil
}
