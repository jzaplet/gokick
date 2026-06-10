package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Short interval + counter + cancel: each job fires at least the immediate
// run-once tick plus a few ticker ticks before the test cancels.
func TestScheduler_RunsAndStops(t *testing.T) {
	t.Parallel()

	var aCount, bCount int32
	s, err := NewScheduler(silentLogger(), []Job{
		{Name: "a", Interval: 10 * time.Millisecond, Fn: func(_ context.Context) error {
			atomic.AddInt32(&aCount, 1)
			return nil
		}},
		{Name: "b", Interval: 10 * time.Millisecond, Fn: func(_ context.Context) error {
			atomic.AddInt32(&bCount, 1)
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	time.Sleep(55 * time.Millisecond) // run-once + ~5 ticks per job
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler did not drain after ctx cancel")
	}

	a := atomic.LoadInt32(&aCount)
	b := atomic.LoadInt32(&bCount)
	if a < 2 || b < 2 {
		t.Fatalf("counts too low (run-once-then-tick should give >=2): a=%d b=%d", a, b)
	}
}

func TestNewScheduler_DuplicateName(t *testing.T) {
	t.Parallel()

	noop := func(_ context.Context) error { return nil }
	_, err := NewScheduler(silentLogger(), []Job{
		{Name: "dup", Interval: time.Second, Fn: noop},
		{Name: "dup", Interval: time.Second, Fn: noop},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestNewScheduler_InvalidJob(t *testing.T) {
	t.Parallel()

	noop := func(_ context.Context) error { return nil }
	cases := []struct {
		name string
		jobs []Job
		want string
	}{
		{"empty name", []Job{{Name: "", Interval: time.Second, Fn: noop}}, "name is required"},
		{"zero interval", []Job{{Name: "x", Interval: 0, Fn: noop}}, "non-positive interval"},
		{"nil fn", []Job{{Name: "x", Interval: time.Second, Fn: nil}}, "nil Fn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewScheduler(silentLogger(), tc.jobs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// A panicking job must not take down its siblings — recovery is per-tick.
func TestScheduler_PanicInOneJobKeepsOthersRunning(t *testing.T) {
	t.Parallel()

	var safeRuns int32
	var panicRuns int32

	s, err := NewScheduler(silentLogger(), []Job{
		{Name: "safe", Interval: 10 * time.Millisecond, Fn: func(_ context.Context) error {
			atomic.AddInt32(&safeRuns, 1)
			return nil
		}},
		{Name: "boom", Interval: 10 * time.Millisecond, Fn: func(_ context.Context) error {
			atomic.AddInt32(&panicRuns, 1)
			panic("boom")
		}},
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	time.Sleep(55 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler hung after panic")
	}

	// Both jobs must have ticked multiple times — proves recovery actually
	// let the panicking goroutine come back for the next tick.
	if atomic.LoadInt32(&safeRuns) < 2 {
		t.Fatalf("safe job ticks: %d (want >=2)", safeRuns)
	}
	if atomic.LoadInt32(&panicRuns) < 2 {
		t.Fatalf("panicking job ticks: %d (want >=2) — recovery may be broken", panicRuns)
	}
}

// Job that returns an error keeps running on subsequent ticks (errors are
// logged, not fatal). This is the contract the cleanup job relies on.
func TestScheduler_ErrorReturnedJobKeepsTicking(t *testing.T) {
	t.Parallel()

	var runs int32
	s, err := NewScheduler(silentLogger(), []Job{
		{Name: "flaky", Interval: 10 * time.Millisecond, Fn: func(_ context.Context) error {
			atomic.AddInt32(&runs, 1)
			return errors.New("flaky failure")
		}},
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	time.Sleep(45 * time.Millisecond)
	cancel()
	<-done

	if atomic.LoadInt32(&runs) < 3 {
		t.Fatalf("error-returning job did not keep ticking: %d", runs)
	}
}

// Two independent Scheduler instances running a job with the SAME name both
// tick — there is no multi-instance coordination (no shared lock, no dedup by
// name). This documents the single-process assumption: run two replicas and a
// once-per-interval job fires once PER replica, not once globally. If anyone
// ever bolts on cross-instance locking, the shared counter would stop doubling
// and this test would catch the behavior change.
func TestScheduler_TwoInstancesTickIndependently(t *testing.T) {
	t.Parallel()

	var ticks int32
	newOne := func() *Scheduler {
		s, err := NewScheduler(silentLogger(), []Job{
			{Name: "cleanup", Interval: time.Hour, Fn: func(_ context.Context) error {
				atomic.AddInt32(&ticks, 1)
				return nil
			}},
		})
		if err != nil {
			t.Fatalf("NewScheduler: %v", err)
		}
		return s
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 2)
	for _, s := range []*Scheduler{newOne(), newOne()} {
		go func(s *Scheduler) {
			defer func() { done <- struct{}{} }()
			s.Run(ctx)
		}(s)
	}

	// A long Interval means only the run-once tick fires before we cancel, so
	// the count is exactly "one per instance" — no coordination would let it be 2.
	time.Sleep(40 * time.Millisecond)
	cancel()
	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("a scheduler instance did not drain after cancel")
		}
	}

	if got := atomic.LoadInt32(&ticks); got != 2 {
		t.Fatalf(
			"two instances should each run the job once (no coordination): got %d ticks, want 2",
			got,
		)
	}
}

// Cancel arrives after the first run-once tick completed; ctx.Done() must
// preempt the long ticker wait so the goroutine exits without leaking.
func TestScheduler_CancelDuringTickerWait(t *testing.T) {
	t.Parallel()

	ran := make(chan struct{})
	once := sync.Once{}
	s, err := NewScheduler(silentLogger(), []Job{
		{Name: "x", Interval: time.Hour, Fn: func(_ context.Context) error {
			once.Do(func() { close(ran) })
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	<-ran // run-once-then-tick guarantees the first invocation happens
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler hung after early cancel — ticker.C should yield to ctx.Done()")
	}
}
