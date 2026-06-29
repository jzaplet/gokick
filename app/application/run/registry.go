// Package run holds the application-side contract for durable runs: the handler
// signature the app implements, the Checkpointer it is given, and the registry
// that maps a run kind to its handler + lease. The worker (infrastructure) drives
// these; the agent's own loop lives in the app handler.
package run

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gokick/app/domain/run"
)

// ErrLeaseLost is returned by Checkpointer.Save when the run was reclaimed by
// another worker (Checkpoint reported ownership lost). A handler that sees it —
// directly or via errors.Is — must stop promptly; its ctx is also cancelled.
var ErrLeaseLost = errors.New("run: lease lost (reclaimed by another worker)")

// Checkpointer persists resumable state mid-run. The worker binds it to the
// claimed run's id/owner/lease; the handler just calls Save with its latest state.
type Checkpointer interface {
	// Save persists state and renews the lease. Returns ErrLeaseLost if ownership
	// was lost (the handler must stop) or another error on a write failure.
	Save(ctx context.Context, state []byte) error
}

// HandlerFunc is one durable run invocation. r is READ-ONLY post-claim — resume
// from r.State (nil/empty = from scratch) and r.Payload; never mutate r (the
// worker's finalize reads r.Attempts/r.Reclaims). Persist progress via ck.Save.
// ctx is cancelled on lease loss, an operator cancel, or worker shutdown — the
// handler must be ctx-aware and return promptly when it fires. A run cancelled
// mid-flight is recorded Cancelled regardless of the handler's return value, so a
// handler need not distinguish a clean-stop nil from a ctx.Err() on cancellation.
type HandlerFunc func(ctx context.Context, r *run.Run, ck Checkpointer) error

// Registration binds a kind to its handler, an optional per-kind lease (the
// crash-reclaim window / heartbeat target; zero = registry default), and an optional
// per-attempt Timeout (zero = no timeout). Build one with FireAndForget (a job-like
// task) or Durable (a long, resumable task) rather than the struct literal.
type Registration struct {
	Handler HandlerFunc
	Lease   time.Duration
	Timeout time.Duration
}

// FireAndForget registers a short, non-resumable task — the "job" shape: the default
// lease, an optional per-attempt timeout, and a handler that need not checkpoint.
func FireAndForget(handler HandlerFunc, timeout time.Duration) Registration {
	return Registration{Handler: handler, Timeout: timeout}
}

// Durable registers a long, resumable task — the "run" shape: an explicit crash-reclaim
// lease and an optional per-attempt timeout. The handler checkpoints via ck.Save.
func Durable(handler HandlerFunc, lease, timeout time.Duration) Registration {
	return Registration{Handler: handler, Lease: lease, Timeout: timeout}
}

// minKindLease rejects an implausibly small explicit per-kind lease at registration
// rather than letting it fail obscurely at runtime (the worker would derive a sub-ms
// heartbeat, the run would thrash claim→instant-expiry→reclaim until poison-failing).
// Durable runs last minutes to hours, so a sub-millisecond lease is always a typo —
// e.g. Lease: 1 (1ns) intending time.Millisecond. Surfaces the mistake loudly at boot.
const minKindLease = time.Millisecond

// HandlerRegistry maps run Kind → Registration. Populated once at DI; read-only
// at runtime.
type HandlerRegistry struct {
	regs         map[string]Registration
	defaultLease time.Duration
}

func NewHandlerRegistry(
	regs map[string]Registration,
	defaultLease time.Duration,
) (*HandlerRegistry, error) {
	if defaultLease <= 0 {
		return nil, fmt.Errorf("run: registry default lease must be > 0 (got %s)", defaultLease)
	}
	dup := make(map[string]Registration, len(regs))
	for kind, reg := range regs {
		if kind == "" {
			return nil, fmt.Errorf("run: empty kind in registry")
		}
		if reg.Handler == nil {
			return nil, fmt.Errorf("run: nil handler for kind %q", kind)
		}
		// A set-but-tiny per-kind lease is a typo (Lease <= 0 means "use default" and
		// is handled by Lookup; only a positive sub-ms value is rejected here).
		if reg.Lease > 0 && reg.Lease < minKindLease {
			return nil, fmt.Errorf(
				"run: kind %q lease %s is implausibly small (min %s) — likely a units typo",
				kind, reg.Lease, minKindLease)
		}
		dup[kind] = reg
	}
	return &HandlerRegistry{regs: dup, defaultLease: defaultLease}, nil
}

// Lookup returns the handler, the effective lease (the kind's, or the default), the
// per-attempt timeout (0 = none), and whether the kind is registered.
func (r *HandlerRegistry) Lookup(kind string) (HandlerFunc, time.Duration, time.Duration, bool) {
	reg, ok := r.regs[kind]
	if !ok {
		return nil, 0, 0, false
	}
	lease := reg.Lease
	if lease <= 0 {
		lease = r.defaultLease
	}
	return reg.Handler, lease, reg.Timeout, true
}

func (r *HandlerRegistry) Has(kind string) bool {
	_, ok := r.regs[kind]
	return ok
}

// Kinds returns the registered kinds in stable order (for startup logs).
func (r *HandlerRegistry) Kinds() []string {
	out := make([]string, 0, len(r.regs))
	for k := range r.regs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
