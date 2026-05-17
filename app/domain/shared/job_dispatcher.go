package shared

import (
	"context"
	"time"
)

// JobDispatcher enqueues background jobs from inside a command/event handler.
// Implementation lives in infrastructure but the interface stays in domain so
// handlers depend on it without importing application/job.
//
// maxRetries is a required positional parameter (not an option) — choosing a
// retry count is a per-kind decision, not something to default. 0 means "run
// once, no retry"; higher for flaky external work (e.g. 3 = up to 3 retries
// after the first failure, so 4 attempts total).
//
// When called from a CommandBus handler (inside a transaction), the enqueue
// write joins the same transaction — business write and job enqueue commit
// atomically. From an event handler (post-commit) or CLI, the enqueue runs
// in its own statement.
type JobDispatcher interface {
	Enqueue(
		ctx context.Context,
		kind string,
		maxRetries int,
		payload any,
		opts ...EnqueueOption,
	) error
}

type EnqueueOptions struct {
	Delay time.Duration // 0 = run as soon as possible
}

type EnqueueOption func(*EnqueueOptions)

func WithDelay(d time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) { o.Delay = d }
}

type jobDispatcherKeyType struct{}

var jobDispatcherKey = jobDispatcherKeyType{}

func ContextWithJobDispatcher(ctx context.Context, d JobDispatcher) context.Context {
	return context.WithValue(ctx, jobDispatcherKey, d)
}

// JobDispatcherFromContext returns the dispatcher injected by the bus or
// worker middleware. Outside both (e.g. tests, CLI bypass) it returns a
// no-op dispatcher so handlers never nil-check; enqueue calls are silently
// dropped.
func JobDispatcherFromContext(ctx context.Context) JobDispatcher {
	if d, ok := ctx.Value(jobDispatcherKey).(JobDispatcher); ok {
		return d
	}
	return noopJobDispatcher{}
}

type noopJobDispatcher struct{}

func (noopJobDispatcher) Enqueue(context.Context, string, int, any, ...EnqueueOption) error {
	return nil
}
