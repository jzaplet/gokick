package run_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/internal/testfx"
)

func TestRequestCancel_SetsFlag_OnClaimedRun_NoOwnerNeeded(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_set.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	claimAs(t, fx, newOwner("wA")) // owned by A; operator cancels without an owner token

	if err := fx.Runs.RequestCancel(ctx, r.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	if got := mustFind(t, fx, r.ID); !got.CancelRequested {
		t.Fatal("cancel_requested must be set after RequestCancel")
	}
}

func TestRequestCancel_Idempotent(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_idem.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	for i := 0; i < 3; i++ {
		if err := fx.Runs.RequestCancel(ctx, r.ID); err != nil {
			t.Fatalf("request cancel #%d: %v", i, err)
		}
	}
	if got := mustFind(t, fx, r.ID); !got.CancelRequested {
		t.Fatal("cancel_requested must remain set")
	}
}

func TestRequestCancel_OnCompleted_NoOp(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_completed.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	if ok, _ := fx.Runs.MarkComplete(ctx, r.ID, owner); !ok {
		t.Fatal("complete should succeed")
	}

	// RequestCancel on a terminal run is a no-op, not an error.
	if err := fx.Runs.RequestCancel(ctx, r.ID); err != nil {
		t.Fatalf("request cancel on completed must not error: %v", err)
	}
	got := mustFind(t, fx, r.ID)
	if got.CancelRequested {
		t.Fatal("a completed run must not become cancel_requested")
	}
	if got.CompletedAt == nil {
		t.Fatal("completion must be untouched")
	}
}

// cancel_requested does NOT make a run unclaimable (only cancelled_at does), and
// the signal rides on the row — a worker reclaiming a crashed run honors it.
func TestCancelRequested_StillClaimable_SurvivesReclaim(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_reclaim.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	claimAs(t, fx, newOwner("wA"))
	if err := fx.Runs.RequestCancel(ctx, r.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	forceExpire(t, fx, r.ID)

	b, err := fx.Runs.ClaimDue(ctx, newOwner("wB"), testLease)
	if err != nil || b == nil {
		t.Fatalf("a cancel-requested run must still be reclaimable: %v / %v", b, err)
	}
	if !b.CancelRequested {
		t.Fatal("the reclaiming worker must see the pending cancel signal on the claimed run")
	}
}

func TestMarkCancelled_SetsCancelledClearsLock_PreservesState(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_mark.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	_, _ = fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":4}`), testLease)

	ok, err := fx.Runs.MarkCancelled(ctx, r.ID, owner)
	if err != nil || !ok {
		t.Fatalf("mark cancelled: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if got.CancelledAt == nil {
		t.Fatal("cancelled_at must be set")
	}
	if got.CompletedAt != nil || got.FailedAt != nil {
		t.Fatal("cancellation is a distinct terminal state")
	}
	if got.LockedUntil != nil || got.LockedBy != nil {
		t.Fatal("cancel must clear the lock")
	}
	if string(got.State) != `{"step":4}` {
		t.Fatalf("cancellation must preserve the last checkpoint: got %q", got.State)
	}
	if next := claimAs(t, fx, newOwner("wB")); next != nil {
		t.Fatal("a cancelled run must not be claimable")
	}
}

func TestMarkCancelled_WrongOwner_False(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_wrong.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	claimAs(t, fx, newOwner("wA"))

	ok, err := fx.Runs.MarkCancelled(ctx, r.ID, newOwner("wWrong"))
	if err != nil || ok {
		t.Fatalf("MarkCancelled wrong-owner: ok=%v err=%v want false,nil", ok, err)
	}
	if got := mustFind(t, fx, r.ID); got.CancelledAt != nil {
		t.Fatal("a non-owner must not cancel the run")
	}
}

func TestMarkCancelled_AfterReclaim_StaleFenced(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_stale.db"))
	ctx := context.Background()
	id, ownerA, ownerB := reclaimedByB(t, fx)

	ok, err := fx.Runs.MarkCancelled(ctx, id, ownerA)
	if err != nil || ok {
		t.Fatalf("stale MarkCancelled: ok=%v err=%v want false,nil", ok, err)
	}
	got := mustFind(t, fx, id)
	if got.CancelledAt != nil {
		t.Fatal("a stalled worker must not cancel a run B now owns")
	}
	if got.LockedBy == nil || *got.LockedBy != ownerB {
		t.Fatal("run must remain B's")
	}
}

// A cancelled run is terminal: not claimable, and every owner-checked write fails.
func TestCancelled_IsTerminal_GuardsAndClaim(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "cancel_terminal.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	if ok, _ := fx.Runs.MarkCancelled(ctx, r.ID, owner); !ok {
		t.Fatal("cancel should succeed")
	}

	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"x":1}`), testLease); err != nil ||
		ok {
		t.Fatalf("checkpoint after cancel: ok=%v err=%v want false,nil", ok, err)
	}
	if ok, err := fx.Runs.MarkComplete(ctx, r.ID, owner); err != nil || ok {
		t.Fatalf("complete after cancel: ok=%v err=%v want false,nil", ok, err)
	}
	if ok, err := fx.Runs.RenewLease(ctx, r.ID, owner, testLease); err != nil || ok {
		t.Fatalf("renew after cancel: ok=%v err=%v want false,nil", ok, err)
	}
	if ok, err := fx.Runs.MarkCancelled(ctx, r.ID, owner); err != nil || ok {
		t.Fatalf("double-cancel: ok=%v err=%v want false,nil", ok, err)
	}
	if next := claimAs(t, fx, newOwner("wB")); next != nil {
		t.Fatal("a cancelled run must not be claimable")
	}
}

// IsCancelRequested reads only the cancel flag, across all three branches: false
// before any request, true after RequestCancel, and (false, nil) for an absent run
// — the heartbeat polls by id, and a row that vanished (reclaimed elsewhere then
// finalized) must read as "no cancel", never error out and break the renew loop.
func TestIsCancelRequested_AllBranches(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "iscancel.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")

	// Before any request → false.
	if got, err := fx.Runs.IsCancelRequested(ctx, r.ID); err != nil || got {
		t.Fatalf("before RequestCancel: got=%v err=%v, want false/nil", got, err)
	}
	// After RequestCancel → true.
	if err := fx.Runs.RequestCancel(ctx, r.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	if got, err := fx.Runs.IsCancelRequested(ctx, r.ID); err != nil || !got {
		t.Fatalf("after RequestCancel: got=%v err=%v, want true/nil", got, err)
	}
	// Absent run → (false, nil), NOT an error. newOwner yields an arbitrary
	// uuid-suffixed string that matches no row.
	if got, err := fx.Runs.IsCancelRequested(ctx, newOwner("ghost")); err != nil || got {
		t.Fatalf("absent run: got=%v err=%v, want (false, nil)", got, err)
	}
}
