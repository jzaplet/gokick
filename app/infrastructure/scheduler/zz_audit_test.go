package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a concurrency-safe sink for slog output. Run launches jobs in
// their own goroutines, so the handler writes from several goroutines; the
// mutex keeps the race detector happy. Tests only read String() after the
// scheduler has drained (<-done), which is the real happens-before edge.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLogger(w *lockedBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, nil))
}

// infra-sched-job-04: on startup the scheduler logs "scheduler: starting"
// including the number of registered jobs. Registering three jobs must surface
// jobs=3 in the line — asserting the count, not just the message, so a
// regression that dropped len(s.jobs) from the log would fail.
func TestScheduler_StartupLogReportsJobCount(t *testing.T) {
	t.Parallel()

	noop := func(_ context.Context) error { return nil }
	buf := &lockedBuffer{}
	// Long interval: each job run-once-ticks exactly once, then waits on the
	// ticker until cancel — no extra noise in the log.
	s, err := NewScheduler(captureLogger(buf), []Job{
		{Name: "a", Interval: time.Hour, Fn: noop},
		{Name: "b", Interval: time.Hour, Fn: noop},
		{Name: "c", Interval: time.Hour, Fn: noop},
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
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not drain")
	}

	out := buf.String()
	if !strings.Contains(out, "scheduler: starting") {
		t.Fatalf("missing startup line; got:\n%s", out)
	}
	// Three jobs were registered → the count in the line must be 3.
	if !strings.Contains(out, "jobs=3") {
		t.Fatalf("startup line should report jobs=3; got:\n%s", out)
	}
}

// infra-sched-job-05: a successful tick logs "scheduler: job completed" with the
// job name and the execution duration. Long interval + a signal channel closed
// inside Fn means exactly one tick (the run-once) completes before we cancel, so
// the assertion targets a single deterministic completion line. The duration
// value is non-deterministic, so we assert the duration_ms= key is present, not a
// specific value.
func TestScheduler_CompletedTickLogsNameAndDuration(t *testing.T) {
	t.Parallel()

	ran := make(chan struct{})
	once := sync.Once{}
	buf := &lockedBuffer{}
	s, err := NewScheduler(captureLogger(buf), []Job{
		{Name: "cleanup", Interval: time.Hour, Fn: func(_ context.Context) error {
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

	<-ran // run-once guarantees Fn executed; its completion line is now logged
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not drain")
	}

	out := buf.String()
	if !strings.Contains(out, "scheduler: job completed") {
		t.Fatalf("missing completion line; got:\n%s", out)
	}
	if !strings.Contains(out, `name=cleanup`) {
		t.Fatalf("completion line should carry the job name; got:\n%s", out)
	}
	if !strings.Contains(out, "duration_ms=") {
		t.Fatalf("completion line should carry the execution duration; got:\n%s", out)
	}
}
