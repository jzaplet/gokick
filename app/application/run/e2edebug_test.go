package run

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
)

// recordingCheckpointer records each Save, or returns a preset error (e.g.
// ErrLeaseLost) to exercise the stop-on-lost-lease path.
type recordingCheckpointer struct {
	states [][]byte
	err    error
}

func (c *recordingCheckpointer) Save(_ context.Context, state []byte) error {
	if c.err != nil {
		return c.err
	}
	cp := make([]byte, len(state))
	copy(cp, state)
	c.states = append(c.states, cp)
	return nil
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func e2eHandler(t *testing.T, kind string) HandlerFunc {
	t.Helper()
	reg, ok := E2EDebugRegistrations()[kind]
	if !ok {
		t.Fatalf("kind %q not registered", kind)
	}
	return reg.Handler
}

func TestE2EDebug_Succeed_ReturnsNil(t *testing.T) {
	t.Parallel()
	if err := e2eHandler(t, E2EKindSucceed)(context.Background(), &run.Run{}, &recordingCheckpointer{}); err != nil {
		t.Fatalf("succeed must return nil, got %v", err)
	}
}

func TestE2EDebug_Fail_ReturnsError(t *testing.T) {
	t.Parallel()
	if err := e2eHandler(t, E2EKindFail)(context.Background(), &run.Run{}, &recordingCheckpointer{}); err == nil {
		t.Fatal("fail must return an error so the run exhausts retries and fails terminally")
	}
}

// From scratch (no state), the checkpoint handler writes exactly Steps checkpoints.
func TestE2EDebug_Checkpoint_RunsAllStepsFromScratch(t *testing.T) {
	t.Parallel()
	ck := &recordingCheckpointer{}
	rn := &run.Run{Payload: mustJSON(t, E2ECheckpointPayload{Steps: 4})}
	if err := e2eHandler(t, E2EKindCheckpoint)(context.Background(), rn, ck); err != nil {
		t.Fatalf("checkpoint handler: %v", err)
	}
	if len(ck.states) != 4 {
		t.Fatalf("want 4 checkpoints, got %d", len(ck.states))
	}
	var last e2eCheckpointState
	if err := json.Unmarshal(ck.states[len(ck.states)-1], &last); err != nil {
		t.Fatalf("decode last state: %v", err)
	}
	if last.Step != 4 {
		t.Fatalf("last checkpoint step: got %d want 4", last.Step)
	}
}

// THE crash-recovery property, unit-level: given a mid-run checkpoint (Step 3 of
// 5), a fresh invocation resumes and runs ONLY the remaining steps (4, 5) — it
// does not restart from zero. The E2E variant proves the same across a real
// process kill; this pins the handler contract it rests on.
func TestE2EDebug_Checkpoint_ResumesFromState(t *testing.T) {
	t.Parallel()
	ck := &recordingCheckpointer{}
	rn := &run.Run{
		Payload: mustJSON(t, E2ECheckpointPayload{Steps: 5}),
		State:   mustJSON(t, e2eCheckpointState{Step: 3}),
	}
	if err := e2eHandler(t, E2EKindCheckpoint)(context.Background(), rn, ck); err != nil {
		t.Fatalf("checkpoint handler: %v", err)
	}
	if len(ck.states) != 2 {
		t.Fatalf(
			"resume must run only the remaining steps: got %d checkpoints want 2",
			len(ck.states),
		)
	}
}

func TestE2EDebug_Checkpoint_StopsOnLeaseLost(t *testing.T) {
	t.Parallel()
	ck := &recordingCheckpointer{err: ErrLeaseLost}
	rn := &run.Run{Payload: mustJSON(t, E2ECheckpointPayload{Steps: 5})}
	err := e2eHandler(t, E2EKindCheckpoint)(context.Background(), rn, ck)
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("must propagate ErrLeaseLost so the worker stops, got %v", err)
	}
}

func TestE2EDebug_Sleep_ReturnsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e2eHandler(t, E2EKindSleep)(ctx, &run.Run{}, &recordingCheckpointer{}) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("sleep must return ctx.Err() on cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sleep did not return after cancel")
	}
}

// recordingDispatcher records the kinds a handler enqueues (the effect handler's
// observable side-effect).
type recordingDispatcher struct{ kinds []string }

func (d *recordingDispatcher) Enqueue(
	_ context.Context, kind string, _ int, _ any, _ ...shared.EnqueueOption,
) error {
	d.kinds = append(d.kinds, kind)
	return nil
}

// The effect handler fires its side-effect (enqueue a marker) BEFORE the sleep, so a
// crash in the sleep window still leaves the effect done — the at-least-once window.
func TestE2EDebug_Effect_EnqueuesMarkerBeforeSleep(t *testing.T) {
	t.Parallel()
	disp := &recordingDispatcher{}
	ctx := shared.ContextWithRunDispatcher(context.Background(), disp)
	rn := &run.Run{Payload: mustJSON(t, map[string]int{"sleep_ms": 0})}
	if err := e2eHandler(t, E2EKindEffect)(ctx, rn, &recordingCheckpointer{}); err != nil {
		t.Fatalf("effect handler: %v", err)
	}
	if len(disp.kinds) != 1 || disp.kinds[0] != E2EKindSucceed {
		t.Fatalf("effect must enqueue exactly one %q marker, got %v", E2EKindSucceed, disp.kinds)
	}
}

func TestE2EDebug_Effect_ReturnsOnCancelAfterEffect(t *testing.T) {
	t.Parallel()
	disp := &recordingDispatcher{}
	ctx, cancel := context.WithCancel(shared.ContextWithRunDispatcher(context.Background(), disp))
	rn := &run.Run{Payload: mustJSON(t, map[string]int{"sleep_ms": 60000})}
	done := make(chan error, 1)
	go func() { done <- e2eHandler(t, E2EKindEffect)(ctx, rn, &recordingCheckpointer{}) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("effect must return ctx.Err() on cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("effect did not return on cancel")
	}
	if len(disp.kinds) != 1 {
		t.Fatalf("marker must be enqueued before the sleep, got %v", disp.kinds)
	}
}
