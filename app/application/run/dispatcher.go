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
	// Validate the RunDispatcher API contract at this service boundary: a rich,
	// operation-named error and an early exit before marshaling. run.NewRun
	// re-enforces the same maxRetries/kind invariant as the entity's own
	// unbypassable chokepoint (a second enqueue path, debug_run, exists) — this
	// check is the friendly front door, not the guard.
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

	r, err := run.NewRun(kind, raw, maxRetries)
	if err != nil {
		return fmt.Errorf("run: construct %q: %w", kind, err)
	}
	// Stamp the tenant from the enqueuing context so the worker can restore it for
	// the handler (the worker bypasses the bus). The empty case (a non-bus enqueue)
	// is resolved fail-closed by the repo (RequireTenant): default in single-tenant,
	// error in multitenant — so a run is never silently born in the default tenant.
	r.TenantID = shared.TenantIDFromContext(ctx)
	// Stamp the language the same way — this is the ENQUEUING request's (the
	// actor's) language, so the run keeps speaking it long after the request
	// died; a run addressed to a DIFFERENT user loads that user's preference
	// itself.
	r.Lang = string(shared.LangFromContext(ctx))
	if options.Delay > 0 {
		r.RunAt = time.Now().Add(options.Delay)
	}
	return d.repo.Enqueue(ctx, r)
}
