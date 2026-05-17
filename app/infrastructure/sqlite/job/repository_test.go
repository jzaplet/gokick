package job_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gokick/app/domain/job"
	"gokick/app/internal/testfx"
)

func enqueueJob(t *testing.T, fx *testfx.Fixture, kind string) *job.Job {
	t.Helper()
	j := job.NewJob(kind, []byte(`{}`), 3)
	if err := fx.Jobs.Enqueue(context.Background(), j); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return j
}

func TestRepository_EnqueueAndFind(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_enqueue.db"))
	j := enqueueJob(t, fx, "test:kind")

	got, err := fx.Jobs.FindByID(context.Background(), j.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("expected job persisted")
	}
	if got.Kind != "test:kind" {
		t.Fatalf("kind: got %q want test:kind", got.Kind)
	}
	if got.Attempts != 0 {
		t.Fatalf("attempts: got %d want 0", got.Attempts)
	}
	if got.MaxRetries != 3 {
		t.Fatalf("max_retries: got %d want 3", got.MaxRetries)
	}
}

func TestRepository_ClaimDue_EmptyReturnsNil(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_empty.db"))

	got, err := fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil from empty queue, got %+v", got)
	}
}

func TestRepository_ClaimDue_PicksOldestPending(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_claim.db"))
	enqueueJob(t, fx, "first")
	// Brief gap so run_at differs measurably.
	time.Sleep(1100 * time.Millisecond)
	enqueueJob(t, fx, "second")

	claimed, err := fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a job")
	}
	if claimed.Kind != "first" {
		t.Fatalf("kind: got %q want first (ORDER BY run_at)", claimed.Kind)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("attempts: got %d want 1 (claim bumps attempts)", claimed.Attempts)
	}
	if claimed.LockedUntil == nil {
		t.Fatal("locked_until must be set after claim")
	}
}

// Atomicity: parallel claim attempts must each get a distinct job (or nil).
// SQLite serializes writers, so this also implicitly verifies the
// UPDATE … RETURNING is atomic across connections.
func TestRepository_ClaimDue_AtomicConcurrent(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_concurrent.db"))
	const total = 20
	for i := 0; i < total; i++ {
		enqueueJob(t, fx, "parallel")
	}

	var (
		mu      sync.Mutex
		claimed = map[string]int{}
		nils    int32
	)

	var wg sync.WaitGroup
	for i := 0; i < total*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := fx.Jobs.ClaimDue(context.Background(), time.Minute)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if j == nil {
				atomic.AddInt32(&nils, 1)
				return
			}
			mu.Lock()
			claimed[j.ID]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(claimed) != total {
		t.Fatalf("distinct claims: got %d want %d", len(claimed), total)
	}
	for id, count := range claimed {
		if count != 1 {
			t.Fatalf("job %s claimed %d times (must be exactly 1)", id, count)
		}
	}
}

func TestRepository_ClaimDue_SkipsLocked(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_locked.db"))
	enqueueJob(t, fx, "only")

	first, _ := fx.Jobs.ClaimDue(context.Background(), time.Hour)
	if first == nil {
		t.Fatal("first claim should succeed")
	}

	second, err := fx.Jobs.ClaimDue(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if second != nil {
		t.Fatal("locked job must not be claimable")
	}
}

func TestRepository_MarkComplete(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_complete.db"))
	j := enqueueJob(t, fx, "x")
	claimed, _ := fx.Jobs.ClaimDue(context.Background(), time.Minute)

	if err := fx.Jobs.MarkComplete(context.Background(), claimed.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.CompletedAt == nil {
		t.Fatal("completed_at must be set")
	}
	if got.LockedUntil != nil {
		t.Fatal("locked_until must be cleared on complete")
	}

	// Completed job is no longer claimable.
	next, _ := fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if next != nil {
		t.Fatal("completed job must not appear in claim again")
	}
}

func TestRepository_Reschedule(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_reschedule.db"))
	j := enqueueJob(t, fx, "y")
	_, _ = fx.Jobs.ClaimDue(context.Background(), time.Minute)

	future := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if err := fx.Jobs.Reschedule(context.Background(), j.ID, future, "boom"); err != nil {
		t.Fatalf("reschedule: %v", err)
	}

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.LastError == nil || *got.LastError != "boom" {
		t.Fatalf("last_error: got %v want boom", got.LastError)
	}
	if got.LockedUntil != nil {
		t.Fatal("locked_until must be cleared on reschedule")
	}
	if !got.RunAt.UTC().Truncate(time.Second).Equal(future) {
		t.Fatalf("run_at: got %v want %v", got.RunAt, future)
	}

	// Still in future → not claimable.
	picked, _ := fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if picked != nil {
		t.Fatal("future-scheduled job must not be claimable")
	}
}

func TestRepository_MarkFailed(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_failed.db"))
	j := enqueueJob(t, fx, "z")
	_, _ = fx.Jobs.ClaimDue(context.Background(), time.Minute)

	if err := fx.Jobs.MarkFailed(context.Background(), j.ID, "exhausted"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got.FailedAt == nil {
		t.Fatal("failed_at must be set")
	}
	if got.LastError == nil || *got.LastError != "exhausted" {
		t.Fatalf("last_error: %v", got.LastError)
	}

	next, _ := fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if next != nil {
		t.Fatal("failed job must not appear in claim again")
	}
}
