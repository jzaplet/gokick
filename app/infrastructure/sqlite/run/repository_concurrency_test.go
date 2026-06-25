package run_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gokick/app/domain/run"
	"gokick/app/internal/testfx"
)

// ─── Terminal / retry ─────────────────────────────────────────────────────────

func TestMarkComplete_SetsCompletedClearsLock_NotClaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "term_complete.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	if ok, err := fx.Runs.MarkComplete(ctx, r.ID, owner); err != nil || !ok {
		t.Fatalf("complete: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if got.CompletedAt == nil {
		t.Fatal("completed_at must be set")
	}
	if got.LockedUntil != nil || got.LockedBy != nil {
		t.Fatal("complete must clear the lock and owner")
	}
	if next := claimAs(t, fx, newOwner("wB")); next != nil {
		t.Fatal("a completed run must not be claimable")
	}
}

func TestReschedule_ClaimableAgain_BumpsAttempts_ResumesFromCheckpoint(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "term_resched.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	ownerA := newOwner("wA")
	claimAs(t, fx, ownerA)
	if ok, _ := fx.Runs.Checkpoint(ctx, r.ID, ownerA, []byte(`{"step":3}`), testLease); !ok {
		t.Fatal("checkpoint should succeed")
	}

	// Reschedule into the (recent) past so it is immediately claimable again.
	if ok, err := fx.Runs.Reschedule(ctx, r.ID, ownerA, time.Now().Add(-time.Second), "transient"); err != nil ||
		!ok {
		t.Fatalf("reschedule: ok=%v err=%v", ok, err)
	}
	mid := mustFind(t, fx, r.ID)
	if mid.LockedUntil != nil || mid.LockedBy != nil {
		t.Fatal("reschedule must clear the lock and owner")
	}
	if mid.Attempts != 1 {
		t.Fatalf("reschedule must bump attempts to 1, got %d", mid.Attempts)
	}

	b := claimAs(t, fx, newOwner("wB"))
	if b == nil {
		t.Fatal("rescheduled run must be claimable again")
	}
	if string(b.State) != `{"step":3}` {
		t.Fatalf("reschedule preserves checkpoint (resume, not restart): got %q", b.State)
	}
	if b.Attempts != 1 {
		t.Fatalf("attempts: got %d want 1 (claim does not bump it)", b.Attempts)
	}
}

func TestReschedule_FutureNotClaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "term_resched_future.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	if ok, _ := fx.Runs.Reschedule(ctx, r.ID, owner, time.Now().Add(time.Hour), "backoff"); !ok {
		t.Fatal("reschedule should succeed")
	}
	if got := claimAs(t, fx, newOwner("wB")); got != nil {
		t.Fatal("a run rescheduled into the future must not be claimable yet")
	}
}

func TestMarkFailed_Terminal_PreservesState_NotClaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "term_failed.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	_, _ = fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":9}`), testLease)

	if ok, err := fx.Runs.MarkFailed(ctx, r.ID, owner, "unrecoverable"); err != nil || !ok {
		t.Fatalf("markfailed: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if got.FailedAt == nil {
		t.Fatal("failed_at must be set")
	}
	if got.LastError == nil || *got.LastError != "unrecoverable" {
		t.Fatalf("last_error: got %v", got.LastError)
	}
	if string(got.State) != `{"step":9}` {
		t.Fatalf("terminal failure must preserve the last checkpoint: got %q", got.State)
	}
	if got.LockedUntil != nil || got.LockedBy != nil {
		t.Fatal("markfailed must clear the lock")
	}
	if next := claimAs(t, fx, newOwner("wB")); next != nil {
		t.Fatal("a failed run must not be claimable")
	}
}

// ─── Tenant ───────────────────────────────────────────────────────────────────

func TestTenant_PropagatesEnqueueToClaim(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_propagate.db"))
	enqueueRunInTenant(t, fx, "tnt-abc")
	claimed := claimAs(t, fx, newOwner("wA"))
	if claimed == nil || claimed.TenantID != "tnt-abc" {
		t.Fatalf("tenant must ride on the claimed row: got %v", claimed)
	}
}

func TestTenant_ClaimDueGlobalDrainAcrossTenants(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_drain.db"))
	ctx := context.Background()
	// Two runs in distinct tenants, distinct run_at so order is deterministic.
	r1 := run.NewRun("agent", []byte(`{}`), 0)
	r1.TenantID, r1.RunAt = "T1", time.Now().Add(-2*time.Second)
	r2 := run.NewRun("agent", []byte(`{}`), 0)
	r2.TenantID, r2.RunAt = "T2", time.Now().Add(-1*time.Second)
	for _, r := range []*run.Run{r1, r2} {
		if err := fx.Runs.Enqueue(ctx, r); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	c1 := claimAs(t, fx, newOwner("w1"))
	c2 := claimAs(t, fx, newOwner("w2"))
	if c1 == nil || c2 == nil {
		t.Fatal("ClaimDue must drain across tenants (no tenant filter)")
	}
	got := map[string]bool{c1.TenantID: true, c2.TenantID: true}
	if !got["T1"] || !got["T2"] {
		t.Fatalf("global drain must claim both tenants' runs: got %v", got)
	}
}

// ─── Concurrency ──────────────────────────────────────────────────────────────

// N workers draining M runs: every run claimed by exactly one worker — no loss,
// no duplication, no SQLITE_BUSY (busy_timeout absorbs writer serialization).
func TestConcurrency_NWorkersVsMRuns_NoDoubleClaim(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "conc_drain.db"))
	const total = 20
	for i := 0; i < total; i++ {
		enqueueRun(t, fx, "agent")
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
			r, err := fx.Runs.ClaimDue(context.Background(), newOwner("w"), testLease)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if r == nil {
				atomic.AddInt32(&nils, 1)
				return
			}
			mu.Lock()
			claimed[r.ID]++
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
			t.Fatalf("run %s claimed %d times (must be exactly 1)", id, count)
		}
	}
}

func TestConcurrency_TwoOwnersRaceOneRun_OneWins(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "conc_race.db"))
	r := enqueueRun(t, fx, "agent")

	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := fx.Runs.ClaimDue(context.Background(), newOwner("w"), testLease)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if got != nil {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly one owner must win the race, got %d wins", wins)
	}
	if got := mustFind(t, fx, r.ID); got.LockedBy == nil {
		t.Fatal("the winning claim must have stamped an owner")
	}
}

// ─── Edge ─────────────────────────────────────────────────────────────────────

func TestEdge_EmptyPayload_PersistsAndClaims(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "edge_empty_payload.db"))
	ctx := context.Background()
	r := run.NewRun("agent", []byte{}, 0)
	if err := fx.Runs.Enqueue(ctx, r); err != nil {
		t.Fatalf("empty payload must persist (empty != NULL): %v", err)
	}
	got := claimAs(t, fx, newOwner("wA"))
	if got == nil {
		t.Fatal("a run with an empty payload must be claimable")
	}
	if len(got.Payload) != 0 {
		t.Fatalf("empty payload must round-trip empty: got %q", got.Payload)
	}
}

func TestEdge_Reschedule_RunAtMsPrecisionRoundTrips(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "edge_resched_ms.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	// A µs-precision future target must round-trip at ms precision.
	target := time.Now().Add(90 * time.Minute)
	if ok, err := fx.Runs.Reschedule(ctx, r.ID, owner, target, "backoff"); err != nil || !ok {
		t.Fatalf("reschedule: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	wantMs := target.UTC().Truncate(time.Millisecond)
	if !got.RunAt.UTC().Truncate(time.Millisecond).Equal(wantMs) {
		t.Fatalf("run_at must round-trip at ms precision: got %v want %v", got.RunAt, wantMs)
	}
}
