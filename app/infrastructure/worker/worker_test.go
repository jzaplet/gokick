package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jobapp "gokick/app/application/job"
	"gokick/app/domain/job"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func noopDispatcher() shared.JobDispatcher {
	return shared.JobDispatcherFromContext(context.Background())
}

func newWorker(t *testing.T, fx *testfx.Fixture, kind string, fn jobapp.HandlerFunc) *Worker {
	t.Helper()
	registry, err := jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{kind: fn})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return NewWorker(silentLogger(), fx.Jobs, registry, fx.DB, noopDispatcher(), 1)
}

func enqueue(t *testing.T, fx *testfx.Fixture, kind string, maxAttempts int) *job.Job {
	t.Helper()
	j := job.NewJob(kind, []byte(`{}`), maxAttempts)
	if err := fx.Jobs.Enqueue(context.Background(), j); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return j
}

func runOnce(t *testing.T, w *Worker) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	// Give the loop one poll cycle to claim + handle.
	time.Sleep(1100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not drain")
	}
}

func TestWorker_HandlerSuccess_MarksComplete(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_ok.db"))
	var ran int32
	j := enqueue(t, fx, "ok:kind", 3)
	w := newWorker(t, fx, "ok:kind", func(_ context.Context, _ []byte) error {
		atomic.AddInt32(&ran, 1)
		return nil
	})

	runOnce(t, w)

	if atomic.LoadInt32(&ran) != 1 {
		t.Fatalf("handler runs: got %d want 1", ran)
	}
	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt must be set after success")
	}
	if got.LockedUntil != nil {
		t.Fatal("LockedUntil must be cleared")
	}
	if got.Attempts != 1 {
		t.Fatalf("attempts: got %d want 1", got.Attempts)
	}
}

type cascadeEvent struct{}

func (cascadeEvent) EventName() string     { return "cascade.attempt" }
func (cascadeEvent) OccurredAt() time.Time { return time.Now() }

// Helper: build a User the repository can persist without VO ceremony.
func mockUser(id, nickname string) *user.User {
	now := time.Now()
	return &user.User{
		ID: id, Nickname: nickname, PasswordHash: "hash",
		Email: "", Role: "user", Active: true,
		CreatedAt: now, UpdatedAt: now,
	}
}

// Mark-complete-in-handler-tx: when the handler does a DB write (via the
// tx-aware repository) and then succeeds, both the business write and the
// MarkComplete commit atomically.
func TestWorker_HandlerSucceedsWithDBWrite_AllCommitAtomically(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_atomic_ok.db"))
	j := enqueue(t, fx, "writes:user", 3)

	w := newWorker(t, fx, "writes:user", func(ctx context.Context, _ []byte) error {
		return fx.Users.Save(ctx, mockUser("uid-1", "nick-1"))
	})

	runOnce(t, w)

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt must be set")
	}
	saved, _ := fx.Users.FindByNickname(context.Background(), "nick-1")
	if saved == nil {
		t.Fatal("user row must persist after handler success")
	}
}

// Mark-complete-in-handler-tx: handler returns an error → entire transaction
// rolls back. Job is NOT marked complete; the handler's tx-aware DB write is
// also gone. Job becomes claimable again.
func TestWorker_HandlerFails_TxRollsBack_JobStaysClaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_rollback.db"))
	j := enqueue(t, fx, "fails:write", 5)

	w := newWorker(t, fx, "fails:write", func(ctx context.Context, _ []byte) error {
		if err := fx.Users.Save(ctx, mockUser("uid-rollback", "nick-rb")); err != nil {
			return err
		}
		return errors.New("handler failure")
	})

	runOnce(t, w)

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.CompletedAt != nil {
		t.Fatal("CompletedAt must NOT be set on handler failure")
	}
	if got.Attempts != 1 {
		t.Fatalf("attempts: got %d want 1 (claim bumped, failure preserves)", got.Attempts)
	}
	if got.LastError == nil || *got.LastError != "handler failure" {
		t.Fatalf("LastError: got %v want 'handler failure'", got.LastError)
	}
	if got.LockedUntil != nil {
		t.Fatal("LockedUntil must be cleared on Reschedule")
	}

	// Critical: handler's write was rolled back.
	saved, _ := fx.Users.FindByNickname(context.Background(), "nick-rb")
	if saved != nil {
		t.Fatal("rollback failed — handler's user row leaked past failure")
	}
}

func TestWorker_HandlerPanics_Recovers_AndReschedules(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_panic.db"))
	j := enqueue(t, fx, "boom", 5)

	w := newWorker(t, fx, "boom", func(_ context.Context, _ []byte) error {
		panic("disaster")
	})

	runOnce(t, w)

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.CompletedAt != nil {
		t.Fatal("CompletedAt must NOT be set on panic")
	}
	if got.LastError == nil || !strings.Contains(*got.LastError, "panic") {
		t.Fatalf("LastError must mention panic, got %v", got.LastError)
	}
}

// MaxAttempts exhaustion: 1 enqueue with MaxAttempts=1; first failure → mark
// failed (not retry). FailedAt set, job no longer claimable.
func TestWorker_HandlerExhaustsMaxAttempts_MarksFailed(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_exhausted.db"))
	j := enqueue(t, fx, "always:fails", 1)

	w := newWorker(t, fx, "always:fails", func(_ context.Context, _ []byte) error {
		return errors.New("permanent")
	})

	runOnce(t, w)

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.FailedAt == nil {
		t.Fatal("FailedAt must be set after max_attempts reached")
	}
	if got.LastError == nil || *got.LastError != "permanent" {
		t.Fatalf("LastError: got %v", got.LastError)
	}

	// No longer claimable.
	next, _ := fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if next != nil {
		t.Fatal("failed job must not appear in claim again")
	}
}

// Job handler that calls Collect must panic. The worker catches the panic
// and reschedules the job, so the test asserts via the persisted LastError
// rather than recovering inside the handler.
func TestWorker_HandlerCallingCollectPanics(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_cascade.db"))
	j := enqueue(t, fx, "cascade", 5)

	w := newWorker(t, fx, "cascade", func(ctx context.Context, _ []byte) error {
		shared.EventCollectorFromContext(ctx).Collect(cascadeEvent{})
		return nil
	})

	runOnce(t, w)

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.LastError == nil || !strings.Contains(*got.LastError, "Collect called from an event/job handler") {
		t.Fatalf("expected LastError mentioning forbidden Collect, got %v", got.LastError)
	}
}

func TestWorker_UnknownKind_MarksFailed_NoRetry(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_unknown.db"))
	j := enqueue(t, fx, "unregistered", 5)

	// Worker registry only knows "registered" — the enqueued kind is unknown.
	w := newWorker(t, fx, "registered", func(_ context.Context, _ []byte) error {
		return nil
	})

	runOnce(t, w)

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.FailedAt == nil {
		t.Fatal("unknown kind must mark failed")
	}
	if got.LastError == nil || !strings.Contains(*got.LastError, "unknown kind") {
		t.Fatalf("LastError must mention unknown kind, got %v", got.LastError)
	}
}
