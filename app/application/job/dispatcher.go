package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gokick/app/domain/job"
	"gokick/app/domain/shared"
)

// Dispatcher implements shared.JobDispatcher backed by the persistent
// job repository. Enqueue serializes payload to JSON and persists a Job
// via job.Repository — when called inside a transaction (CommandBus handler),
// the INSERT joins the same transaction.
type Dispatcher struct {
	repo     job.Repository
	registry *HandlerRegistry
}

func NewDispatcher(repo job.Repository, registry *HandlerRegistry) *Dispatcher {
	return &Dispatcher{repo: repo, registry: registry}
}

func (d *Dispatcher) Enqueue(
	ctx context.Context,
	kind string,
	maxRetries int,
	payload any,
	opts ...shared.EnqueueOption,
) error {
	if maxRetries < 0 {
		return fmt.Errorf("job: Enqueue(%q) requires maxRetries >= 0 (got %d)", kind, maxRetries)
	}
	if !d.registry.Has(kind) {
		return fmt.Errorf("job: unknown kind %q (handler not registered)", kind)
	}

	options := shared.EnqueueOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("job: marshal payload for kind %q: %w", kind, err)
	}

	j := job.NewJob(kind, raw, maxRetries)
	if options.Delay > 0 {
		j.RunAt = time.Now().Add(options.Delay)
	}
	return d.repo.Enqueue(ctx, j)
}
