package run

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
)

// E2E debug run kinds — registered ONLY when APP_RUN_DEBUG is on (see
// provideRunHandlerRegistry), never in production. They exist so a live or local
// deploy can exercise the durable-run engine's process-lifecycle guarantees that
// an in-process test cannot reach: crash → resume-from-checkpoint, at-least-once
// after a crash, operator cancel, and SIGTERM drain. Enqueued/observed/cancelled
// via the /debug/runs endpoints.
const (
	E2EKindSucceed    = "e2e:succeed"    // completes immediately (also the effect marker)
	E2EKindFail       = "e2e:fail"       // always errors → terminal after maxRetries
	E2EKindCheckpoint = "e2e:checkpoint" // loops N checkpoints; resumes on reclaim
	E2EKindSleep      = "e2e:sleep"      // blocks until ctx is cancelled
	E2EKindEffect     = "e2e:effect"     // fire-and-forget with a side-effect; re-runs whole on reclaim
)

// e2eDebugLease is a deliberately SHORT crash-reclaim lease for the durable e2e
// kinds. Recovery latency ≈ lease: an orphaned run (dead worker) is not
// reclaimable until its lease expires, so a fast crash-recovery test needs a fast
// lease (the production default is minutes). Real kinds set their own.
const e2eDebugLease = 3 * time.Second

// E2ECheckpointPayload configures the checkpoint kind (enqueue-time), E2ECheckpointState
// is its resumable progress (checkpointed). Exported so the /debug/runs endpoint can
// build the payload.
type E2ECheckpointPayload struct {
	Steps   int `json:"steps"`    // number of checkpoints to write (default 5)
	SleepMs int `json:"sleep_ms"` // pause between steps — makes the run long enough to kill
}

type e2eCheckpointState struct {
	Step int `json:"step"`
}

// E2EDebugRegistrations returns the debug run kinds. Merged into the handler
// registry only when APP_RUN_DEBUG is on.
func E2EDebugRegistrations() map[string]Registration {
	return map[string]Registration{
		E2EKindSucceed: FireAndForget(func(context.Context, *run.Run, Checkpointer) error {
			return nil
		}, 30*time.Second),

		E2EKindFail: FireAndForget(func(context.Context, *run.Run, Checkpointer) error {
			return errors.New("e2e: deliberate failure (APP_RUN_DEBUG)")
		}, 30*time.Second),

		E2EKindCheckpoint: Durable(e2eCheckpointHandler, e2eDebugLease, time.Minute),

		E2EKindSleep: Durable(func(ctx context.Context, _ *run.Run, _ Checkpointer) error {
			<-ctx.Done()
			return ctx.Err()
		}, e2eDebugLease, time.Hour),

		E2EKindEffect: FireAndForget(e2eEffectHandler, 30*time.Second),
	}
}

// e2eEffectHandler models a fire-and-forget run with an external side-effect. The
// effect is enqueuing a marker child run (E2EKindSucceed) — it stands in for a real
// effect (mail sent, API called), and unlike an in-memory or file effect it survives
// a crash and is countable in the DB. A fire-and-forget run has NO checkpoint, so a
// reclaim after a crash re-runs the WHOLE handler, firing the effect AGAIN: that is
// at-least-once, and exactly why handlers must be idempotent. It then sleeps a
// window (payload sleep_ms) so a harness can crash it AFTER the effect but BEFORE it
// returns — the gap before the separate MarkComplete write.
func e2eEffectHandler(ctx context.Context, r *run.Run, _ Checkpointer) error {
	// The worker injects the real dispatcher; off-bus it is a no-op (drops silently).
	if err := shared.RunDispatcherFromContext(ctx).Enqueue(ctx, E2EKindSucceed, 0, nil); err != nil {
		return err
	}

	var p struct {
		SleepMs int `json:"sleep_ms"`
	}
	if len(r.Payload) > 0 {
		if err := json.Unmarshal(r.Payload, &p); err != nil {
			return err
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(p.SleepMs) * time.Millisecond):
	}
	return nil
}

// e2eCheckpointHandler counts to Steps, checkpointing after each step and pausing
// SleepMs between them. It RESUMES from the last checkpointed step (r.State), so a
// process killed mid-loop continues from where it stopped instead of restarting —
// the property the crash-recovery E2E asserts. It is ctx-aware (stops on lease
// loss / cancel / shutdown) and idempotent (re-running a step just re-writes it).
func e2eCheckpointHandler(ctx context.Context, r *run.Run, ck Checkpointer) error {
	var p E2ECheckpointPayload
	if len(r.Payload) > 0 {
		if err := json.Unmarshal(r.Payload, &p); err != nil {
			return err
		}
	}
	if p.Steps <= 0 {
		p.Steps = 5
	}

	var st e2eCheckpointState
	if len(r.State) > 0 {
		if err := json.Unmarshal(r.State, &st); err != nil {
			return err
		}
	}

	for st.Step < p.Steps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(p.SleepMs) * time.Millisecond):
		}
		st.Step++
		enc, err := json.Marshal(st)
		if err != nil {
			return err
		}
		if err := ck.Save(ctx, enc); err != nil {
			return err // ErrLeaseLost → stop promptly
		}
	}
	return nil
}
