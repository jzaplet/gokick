package run_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/domain/run"
	"gokick/app/internal/testfx"

	"github.com/google/uuid"
)

func enqueueRunPayload(t *testing.T, fx *testfx.Fixture, payload string) *run.Run {
	t.Helper()
	r := run.NewRun("agent", []byte(payload), 3)
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return r
}

func enqueueRunInTenant(t *testing.T, fx *testfx.Fixture, tenantID string) *run.Run {
	t.Helper()
	r := run.NewRun("agent", []byte(`{}`), 3)
	r.TenantID = tenantID
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return r
}

// reclaimedByB is the standard fencing setup: enqueue a run, claim it as ownerA,
// force the lease to expire, then reclaim it as ownerB. After this, ownerA is the
// STALE worker whose owner-checked writes must all be fenced.
func reclaimedByB(t *testing.T, fx *testfx.Fixture) (id, ownerA, ownerB string) {
	t.Helper()
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	ownerA, ownerB = newOwner("wA"), newOwner("wB")
	if _, err := fx.Runs.ClaimDue(ctx, ownerA, testLease); err != nil {
		t.Fatalf("claim A: %v", err)
	}
	forceExpire(t, fx, r.ID)
	b, err := fx.Runs.ClaimDue(ctx, ownerB, testLease)
	if err != nil || b == nil || b.ID != r.ID || b.LockedBy == nil || *b.LockedBy != ownerB {
		t.Fatalf("reclaim by B failed: b=%v err=%v", b, err)
	}
	return r.ID, ownerA, ownerB
}

// ownerCheckedCall abstracts each owner-checked mutating method so the uniform
// fencing contract can be table-tested across all five.
type ownerCheckedCall struct {
	name string
	call func(ctx context.Context, fx *testfx.Fixture, id, owner string) (bool, error)
}

var ownerCheckedCalls = []ownerCheckedCall{
	{"RenewLease", func(ctx context.Context, fx *testfx.Fixture, id, owner string) (bool, error) {
		return fx.Runs.RenewLease(ctx, id, owner, testLease)
	}},
	{"Checkpoint", func(ctx context.Context, fx *testfx.Fixture, id, owner string) (bool, error) {
		return fx.Runs.Checkpoint(ctx, id, owner, []byte(`{"x":1}`), testLease)
	}},
	{"MarkComplete", func(ctx context.Context, fx *testfx.Fixture, id, owner string) (bool, error) {
		return fx.Runs.MarkComplete(ctx, id, owner)
	}},
	{"Reschedule", func(ctx context.Context, fx *testfx.Fixture, id, owner string) (bool, error) {
		return fx.Runs.Reschedule(ctx, id, owner, time.Now().Add(time.Hour), "boom")
	}},
	{"MarkFailed", func(ctx context.Context, fx *testfx.Fixture, id, owner string) (bool, error) {
		return fx.Runs.MarkFailed(ctx, id, owner, "boom")
	}},
}

// The uniform contract: a fenced-out owner is NOT an error — it is normal lease
// loss. Every owner-checked method returns (false, nil) on a wrong owner.
func TestFence_AllMethods_WrongOwner_FalseNotError(t *testing.T) {
	for _, c := range ownerCheckedCalls {
		t.Run(c.name, func(t *testing.T) {
			fx := testfx.New(t, filepath.Join(t.TempDir(), "fence_wrong_"+c.name+".db"))
			r := enqueueRun(t, fx, "agent")
			claimAs(t, fx, newOwner("wA")) // owned by A
			ok, err := c.call(context.Background(), fx, r.ID, newOwner("wWrong"))
			if err != nil {
				t.Fatalf("%s wrong-owner must NOT error (lease loss is normal): %v", c.name, err)
			}
			if ok {
				t.Fatalf("%s wrong-owner must return false (RowsAffected==0)", c.name)
			}
		})
	}
}

// A missing row must be reported as false, never as an error.
func TestFence_AllMethods_NonexistentID_False(t *testing.T) {
	for _, c := range ownerCheckedCalls {
		t.Run(c.name, func(t *testing.T) {
			fx := testfx.New(t, filepath.Join(t.TempDir(), "fence_missing_"+c.name+".db"))
			ok, err := c.call(context.Background(), fx, uuid.NewString(), newOwner("w"))
			if err != nil || ok {
				t.Fatalf("%s on missing id: ok=%v err=%v want false,nil", c.name, ok, err)
			}
		})
	}
}

// And the positive control: the rightful owner of a live lease gets true.
func TestFence_AllMethods_RightfulOwner_True(t *testing.T) {
	for _, c := range ownerCheckedCalls {
		t.Run(c.name, func(t *testing.T) {
			fx := testfx.New(t, filepath.Join(t.TempDir(), "fence_owner_"+c.name+".db"))
			r := enqueueRun(t, fx, "agent")
			owner := newOwner("wA")
			claimAs(t, fx, owner)
			ok, err := c.call(context.Background(), fx, r.ID, owner)
			if err != nil || !ok {
				t.Fatalf("%s by rightful owner: ok=%v err=%v want true,nil", c.name, ok, err)
			}
		})
	}
}

// ─── Reclaim & resume — the stalled worker (A) is fenced after B reclaims ──────

func TestReclaim_CarriesLastCheckpointState(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "reclaim_state.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	ownerA := newOwner("wA")
	claimAs(t, fx, ownerA)
	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, ownerA, []byte(`{"step":7}`), testLease); err != nil ||
		!ok {
		t.Fatalf("A checkpoint: ok=%v err=%v", ok, err)
	}
	forceExpire(t, fx, r.ID)

	b, err := fx.Runs.ClaimDue(ctx, newOwner("wB"), testLease)
	if err != nil || b == nil {
		t.Fatalf("reclaim: %v / %v", b, err)
	}
	if string(b.State) != `{"step":7}` {
		t.Fatalf("reclaim must carry A's last checkpoint: got %q", b.State)
	}
	if b.Reclaims != 1 {
		t.Fatalf("reclaims: got %d want 1", b.Reclaims)
	}
}

func TestReclaim_FromEmptyState_ResumesFromScratch_PayloadImmutable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "reclaim_scratch.db"))
	ctx := context.Background()
	r := enqueueRunPayload(t, fx, `{"in":"P"}`)
	ownerA := newOwner("wA")
	claimAs(t, fx, ownerA)
	forceExpire(t, fx, r.ID)

	b, err := fx.Runs.ClaimDue(ctx, newOwner("wB"), testLease)
	if err != nil || b == nil {
		t.Fatalf("reclaim: %v / %v", b, err)
	}
	if len(b.State) != 0 {
		t.Fatalf("never-checkpointed reclaim must resume from empty state, got %q", b.State)
	}
	if string(b.Payload) != `{"in":"P"}` {
		t.Fatalf("payload must be immutable across reclaim: got %q", b.Payload)
	}
}

func TestFence_StaleCheckpoint_RejectedAndStateUnchanged(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "stale_ckpt.db"))
	ctx := context.Background()
	id, ownerA, ownerB := reclaimedByB(t, fx)

	// B (current owner) writes a known state.
	if ok, _ := fx.Runs.Checkpoint(ctx, id, ownerB, []byte(`{"by":"B"}`), testLease); !ok {
		t.Fatal("B checkpoint should succeed")
	}
	// A (stale) tries to checkpoint — must be fenced and must NOT overwrite.
	ok, err := fx.Runs.Checkpoint(ctx, id, ownerA, []byte(`{"by":"A"}`), testLease)
	if err != nil || ok {
		t.Fatalf("stale checkpoint: ok=%v err=%v want false,nil", ok, err)
	}
	if got := mustFind(t, fx, id); string(got.State) != `{"by":"B"}` {
		t.Fatalf("stale checkpoint must not overwrite state: got %q", got.State)
	}
}

func TestFence_StaleMarkComplete_RejectedAndNotCompleted(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "stale_complete.db"))
	ctx := context.Background()
	id, ownerA, ownerB := reclaimedByB(t, fx)

	ok, err := fx.Runs.MarkComplete(ctx, id, ownerA)
	if err != nil || ok {
		t.Fatalf("stale complete: ok=%v err=%v want false,nil", ok, err)
	}
	got := mustFind(t, fx, id)
	if got.CompletedAt != nil {
		t.Fatal("a stalled worker must not complete a run B now owns")
	}
	if got.LockedBy == nil || *got.LockedBy != ownerB {
		t.Fatalf("run must still be owned by B: got %v", got.LockedBy)
	}
}

func TestFence_StaleRenewLease_RejectedAndLeaseUntouched(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "stale_renew.db"))
	ctx := context.Background()
	id, ownerA, ownerB := reclaimedByB(t, fx)
	before := mustFind(t, fx, id) // B's lease

	ok, err := fx.Runs.RenewLease(ctx, id, ownerA, 10*time.Hour)
	if err != nil || ok {
		t.Fatalf("stale renew: ok=%v err=%v want false,nil", ok, err)
	}
	after := mustFind(t, fx, id)
	if after.LockedBy == nil || *after.LockedBy != ownerB {
		t.Fatal("stale renew must not steal ownership")
	}
	if !after.LockedUntil.Equal(*before.LockedUntil) {
		t.Fatal("stale renew must not extend B's lease")
	}
}

func TestFence_StaleReschedule_RejectedAndRunUntouched(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "stale_resched.db"))
	ctx := context.Background()
	id, ownerA, ownerB := reclaimedByB(t, fx)
	before := mustFind(t, fx, id)

	ok, err := fx.Runs.Reschedule(ctx, id, ownerA, time.Now().Add(time.Hour), "A backoff")
	if err != nil || ok {
		t.Fatalf("stale reschedule: ok=%v err=%v want false,nil", ok, err)
	}
	got := mustFind(t, fx, id)
	if got.LastError != nil && *got.LastError == "A backoff" {
		t.Fatal("stale reschedule must not set last_error")
	}
	if !got.RunAt.Equal(before.RunAt) {
		t.Fatal("stale reschedule must not bump run_at")
	}
	if got.LockedBy == nil || *got.LockedBy != ownerB {
		t.Fatal("stale reschedule must not unlock B's run")
	}
}

func TestFence_StaleMarkFailed_RejectedAndNotFailed(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "stale_failed.db"))
	ctx := context.Background()
	id, ownerA, ownerB := reclaimedByB(t, fx)

	ok, err := fx.Runs.MarkFailed(ctx, id, ownerA, "A verdict")
	if err != nil || ok {
		t.Fatalf("stale markfailed: ok=%v err=%v want false,nil", ok, err)
	}
	got := mustFind(t, fx, id)
	if got.FailedAt != nil {
		t.Fatal("a stalled worker must not terminally fail B's run")
	}
	if got.LockedBy == nil || *got.LockedBy != ownerB {
		t.Fatal("run must remain B's")
	}
	// And B can still complete it.
	if ok, err := fx.Runs.MarkComplete(ctx, id, ownerB); err != nil || !ok {
		t.Fatalf("B must be able to complete the reclaimed run: ok=%v err=%v", ok, err)
	}
}

// Per-claim tokens let a worker fence against its OWN earlier abandoned attempt:
// a stable per-worker id would equal the new token and wrongly pass.
func TestFence_SelfReclaim_FencesOwnEarlierToken(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "self_reclaim.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	tok1 := newOwner("wSAME")
	claimAs(t, fx, tok1)
	forceExpire(t, fx, r.ID)
	// The SAME worker process reclaims with a FRESH token.
	tok2 := newOwner("wSAME")
	if b, err := fx.Runs.ClaimDue(ctx, tok2, testLease); err != nil || b == nil {
		t.Fatalf("self-reclaim: %v / %v", b, err)
	}
	// Its earlier goroutine (tok1) is now stale and must be fenced.
	ok, err := fx.Runs.Checkpoint(ctx, r.ID, tok1, []byte(`{"stale":true}`), testLease)
	if err != nil || ok {
		t.Fatalf("own stale token must be fenced: ok=%v err=%v", ok, err)
	}
}

func TestReclaim_TenantPreserved(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "reclaim_tenant.db"))
	ctx := context.Background()
	r := enqueueRunInTenant(t, fx, "tnt-123")
	claimAs(t, fx, newOwner("wA"))
	forceExpire(t, fx, r.ID)
	b, err := fx.Runs.ClaimDue(ctx, newOwner("wB"), testLease)
	if err != nil || b == nil {
		t.Fatalf("reclaim: %v / %v", b, err)
	}
	if b.TenantID != "tnt-123" {
		t.Fatalf("tenant must survive reclaim: got %q want tnt-123", b.TenantID)
	}
}

// Terminal guard: once completed (lock cleared), an owner-checked write that only
// keyed on locked_by could still match a NULL — the completed/failed guard closes it.
func TestTerminal_CompleteThenRescheduleRejected(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "terminal_guard.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	if ok, err := fx.Runs.MarkComplete(ctx, r.ID, owner); err != nil || !ok {
		t.Fatalf("complete: ok=%v err=%v", ok, err)
	}
	// A late reschedule (even by the rightful pre-complete owner) must be a no-op.
	ok, err := fx.Runs.Reschedule(ctx, r.ID, owner, time.Now().Add(time.Hour), "late")
	if err != nil || ok {
		t.Fatalf("reschedule after complete: ok=%v err=%v want false,nil", ok, err)
	}
	if got := mustFind(t, fx, r.ID); got.CompletedAt == nil {
		t.Fatal("completed run must stay completed")
	}
}
