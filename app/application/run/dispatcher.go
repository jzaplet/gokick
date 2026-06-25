package run

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
)

// Dispatcher implements shared.RunDispatcher backed by the durable run repository.
// Enqueue serializes the payload to JSON and persists a Run via run.Repository —
// when called inside a CommandBus handler's transaction, the INSERT joins it.
type Dispatcher struct {
	repo     run.Repository
	registry *HandlerRegistry
}

func NewDispatcher(repo run.Repository, registry *HandlerRegistry) *Dispatcher {
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
		return fmt.Errorf("run: Enqueue(%q) requires maxRetries >= 0 (got %d)", kind, maxRetries)
	}
	if !d.registry.Has(kind) {
		return fmt.Errorf("run: unknown kind %q (handler not registered)", kind)
	}

	options := shared.EnqueueOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("run: marshal payload for kind %q: %w", kind, err)
	}

	r := run.NewRun(kind, raw, maxRetries)
	// Stamp the tenant from the enqueuing context so the worker can restore it for
	// the handler (the worker bypasses the bus). Fall back to the default tenant
	// when ctx carries none — an explicit "" would override the column's DEFAULT.
	r.TenantID = shared.TenantIDFromContext(ctx)
	if r.TenantID == "" {
		r.TenantID = shared.DefaultTenantID
	}
	if options.Delay > 0 {
		r.RunAt = time.Now().Add(options.Delay)
	}
	return d.repo.Enqueue(ctx, r)
}
