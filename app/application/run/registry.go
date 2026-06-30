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
// per-attempt Timeout (zero = no timeout). Build one with FireAndForget (a
// fire-and-forget task) or Durable (a long, resumable task) rather than the struct literal.
type Registration struct {
	Handler HandlerFunc
	Lease   time.Duration
	Timeout time.Duration
}

// FireAndForget registers a short, non-resumable task — the fire-and-forget shape: the default
// lease, an optional per-attempt timeout, and a handler that need not checkpoint.
//
// The handler runs OUTSIDE a transaction and is
// marked complete by a SEPARATE write after it returns, so delivery is at-least-once:
// a crash between the handler returning and that write re-runs the handler on reclaim.
// The handler MUST therefore be idempotent (or guard its external effects).
func FireAndForget(handler HandlerFunc, timeout time.Duration) Registration {
	return Registration{Handler: handler, Timeout: timeout}
}

// Durable registers a long, resumable task — the "run" shape: an explicit crash-reclaim
// lease and an optional per-attempt timeout. The handler checkpoints via ck.Save and
// resumes from the last checkpoint on reclaim.
//
// The timeout is PER ATTEMPT and a blown deadline counts as a logic-retry (it bumps
// attempts and is gated by maxRetries), so set it longer than a single resume-to-next-
// checkpoint step — a timeout shorter than the work between checkpoints can fail the
// run terminally after maxRetries even while it is making real progress.
func Durable(handler HandlerFunc, lease, timeout time.Duration) Registration {
	return Registration{Handler: handler, Lease: lease, Timeout: timeout}
}

// minKindLease rejects an implausibly small explicit per-kind lease at registration
// rather than letting it fail obscurely at runtime (the worker would derive a sub-ms
// heartbeat, the run would thrash claim→instant-expiry→reclaim until poison-failing).
// Durable runs last minutes to hours, so a sub-millisecond lease is always a typo —
// e.g. Lease: 1 (1ns) intending time.Millisecond. Surfaces the mistake loudly at boot.
const minKindLease = time.Millisecond

// rejectSubMs flags a positive but implausibly small (< minKindLease) per-kind
// duration as a units typo — shared by the lease and timeout validation, which would
// otherwise repeat the same message. A zero value is "use default / none" (handled by
// Lookup) and passes; the negative case differs per field and stays at the call site.
func rejectSubMs(kind, label string, d time.Duration) error {
	if d > 0 && d < minKindLease {
		return fmt.Errorf(
			"run: kind %q %s %s is implausibly small (min %s) — likely a units typo",
			kind, label, d, minKindLease)
	}
	return nil
}

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
		// A set-but-tiny lease/timeout is a units typo (0 = "use default / none", handled
		// by Lookup; only a positive sub-ms value is rejected). A negative Timeout is a
		// distinct hard error — it would silently disable the timeout the author asked
		// for — so it is checked separately (a negative Lease just means "use default").
		if err := rejectSubMs(kind, "lease", reg.Lease); err != nil {
			return nil, err
		}
		if reg.Timeout < 0 {
			return nil, fmt.Errorf("run: kind %q timeout %s is negative", kind, reg.Timeout)
		}
		if err := rejectSubMs(kind, "timeout", reg.Timeout); err != nil {
			return nil, err
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
