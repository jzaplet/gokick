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
	if b.Reclaims != 0 {
		t.Fatalf(
			"reschedule is a logic retry, not a crash reclaim: reclaims got %d want 0",
			b.Reclaims,
		)
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
	r1, _ := run.NewRun("agent", []byte(`{}`), 0)
	r1.TenantID, r1.RunAt = "T1", time.Now().Add(-2*time.Second)
	r2, _ := run.NewRun("agent", []byte(`{}`), 0)
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

// Two workers racing to reclaim the SAME expired lease: exactly one wins, and the
// reclaims counter is bumped exactly once (the loser's SELECT re-evaluates after
// the winner commits and sees a future locked_until).
func TestConcurrency_TwoOwnersRaceExpiredReclaim_OneWins_ReclaimsOnce(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "conc_reclaim_race.db"))
	r := enqueueRun(t, fx, "agent")
	claimAs(t, fx, newOwner("wA"))
	forceExpire(t, fx, r.ID)

	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := fx.Runs.ClaimDue(context.Background(), newOwner("wReclaim"), testLease)
			if err != nil {
				t.Errorf("reclaim: %v", err)
				return
			}
			if got != nil {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly one worker must reclaim the expired run, got %d", wins)
	}
	if got := mustFind(t, fx, r.ID); got.Reclaims != 1 {
		t.Fatalf("a single reclaim must bump reclaims exactly once, got %d", got.Reclaims)
	}
}

// Every mutating method must bump updated_at: the entity + migration document it,
// and SQLite's DEFAULT CURRENT_TIMESTAMP fires only on INSERT, so each UPDATE must
// set it explicitly.
func TestMutators_AllBumpUpdatedAt(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, fx *testfx.Fixture, id, owner string)
	}{
		{"ClaimDue_reclaim", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			forceExpire(t, fx, id)
			if _, err := fx.Runs.ClaimDue(context.Background(), newOwner("wRe"), testLease); err != nil {
				t.Fatalf("reclaim: %v", err)
			}
		}},
		{"RenewLease", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			if alive, _, err := fx.Runs.RenewLease(context.Background(), id, owner, testLease); err != nil ||
				!alive {
				t.Fatalf("renew: alive=%v err=%v", alive, err)
			}
		}},
		{"Checkpoint", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			if ok, err := fx.Runs.Checkpoint(context.Background(), id, owner, []byte(`{"s":1}`), testLease); err != nil ||
				!ok {
				t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
			}
		}},
		{"MarkComplete", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			if ok, err := fx.Runs.MarkComplete(context.Background(), id, owner); err != nil || !ok {
				t.Fatalf("complete: ok=%v err=%v", ok, err)
			}
		}},
		{"Reschedule", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			if ok, err := fx.Runs.Reschedule(context.Background(), id, owner, time.Now().Add(time.Hour), "e"); err != nil ||
				!ok {
				t.Fatalf("reschedule: ok=%v err=%v", ok, err)
			}
		}},
		{"MarkFailed", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			if ok, err := fx.Runs.MarkFailed(context.Background(), id, owner, "e"); err != nil ||
				!ok {
				t.Fatalf("markfailed: ok=%v err=%v", ok, err)
			}
		}},
		{"RequestCancel", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			// Not owner-checked; touches updated_at on the operator cancel signal.
			if err := fx.Runs.RequestCancel(context.Background(), id); err != nil {
				t.Fatalf("requestcancel: %v", err)
			}
		}},
		{"MarkCancelled", func(t *testing.T, fx *testfx.Fixture, id, owner string) {
			if ok, err := fx.Runs.MarkCancelled(context.Background(), id, owner); err != nil ||
				!ok {
				t.Fatalf("markcancelled: ok=%v err=%v", ok, err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := testfx.New(t, filepath.Join(t.TempDir(), "upd.db"))
			r := enqueueRun(t, fx, "agent")
			owner := newOwner("wA")
			claimAs(t, fx, owner)
			before := mustFind(t, fx, r.ID)
			time.Sleep(5 * time.Millisecond)
			c.mutate(t, fx, r.ID, owner)
			after := mustFind(t, fx, r.ID)
			if !after.UpdatedAt.After(before.UpdatedAt) {
				t.Fatalf(
					"%s must bump updated_at: before=%v after=%v",
					c.name,
					before.UpdatedAt,
					after.UpdatedAt,
				)
			}
		})
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
	r, _ := run.NewRun("agent", []byte{}, 0)
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
