package run_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gokick/app/internal/testfx"
)

func TestCheckpoint_PersistsAndReadBack(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_persist.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	state := []byte(`{"step":1}`)
	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, state, testLease); err != nil || !ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if !bytes.Equal(got.State, state) {
		t.Fatalf("state: got %q want %q", got.State, state)
	}
	if got.LockedBy == nil || *got.LockedBy != owner {
		t.Fatal("checkpoint must keep ownership")
	}
}

func TestCheckpoint_BumpsUpdatedAt(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_updated.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	before := mustFind(t, fx, r.ID)

	time.Sleep(5 * time.Millisecond)
	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"s":1}`), testLease); err != nil ||
		!ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	after := mustFind(t, fx, r.ID)
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf(
			"checkpoint must bump updated_at: before=%v after=%v",
			before.UpdatedAt,
			after.UpdatedAt,
		)
	}
}

func TestCheckpoint_RepeatedOverwrites_LastWriteWins(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_overwrite.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	_, _ = fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":1}`), testLease)
	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":2}`), testLease); err != nil ||
		!ok {
		t.Fatalf("second checkpoint: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if string(got.State) != `{"step":2}` {
		t.Fatalf("state: got %q want last-write-wins {\"step\":2}", got.State)
	}
	if got.Attempts != 0 {
		t.Fatalf("checkpoint must not touch attempts: got %d", got.Attempts)
	}
}

func TestCheckpoint_ManyRepeatsConverge(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_many.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	for i := 0; i < 50; i++ {
		ok, err := fx.Runs.Checkpoint(
			ctx,
			r.ID,
			owner,
			[]byte(fmt.Sprintf(`{"step":%d}`, i)),
			testLease,
		)
		if err != nil || !ok {
			t.Fatalf("checkpoint %d: ok=%v err=%v", i, ok, err)
		}
	}
	if got := mustFind(t, fx, r.ID); string(got.State) != `{"step":49}` {
		t.Fatalf("final state: got %q want {\"step\":49}", got.State)
	}
}

func TestCheckpoint_LargeBlobRoundTrips(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_large.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	big := make([]byte, 2<<20) // 2 MiB
	for i := range big {
		big[i] = byte(i * 31)
	}
	want := sha256.Sum256(big)
	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, big, testLease); err != nil || !ok {
		t.Fatalf("large checkpoint: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if len(got.State) != len(big) {
		t.Fatalf("large state length: got %d want %d", len(got.State), len(big))
	}
	if sha256.Sum256(got.State) != want {
		t.Fatal("large state round-trip corrupted (hash mismatch)")
	}
}

func TestCheckpoint_BinaryWithNUL_RoundTrips(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_binary.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	// Embedded NUL bytes: a TEXT binding would truncate at the first 0x00.
	state := []byte{0x00, 0x01, 0xff, 0x00, 0x42, 0x00}
	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, state, testLease); err != nil || !ok {
		t.Fatalf("binary checkpoint: ok=%v err=%v", ok, err)
	}
	if got := mustFind(t, fx, r.ID); !bytes.Equal(got.State, state) {
		t.Fatalf("binary state must round-trip byte-for-byte: got %v want %v", got.State, state)
	}
}

func TestCheckpoint_EmptyState_AllowedAndRenews(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_empty.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	before := mustFind(t, fx, r.ID)

	time.Sleep(5 * time.Millisecond)
	ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, []byte{}, testLease)
	if err != nil || !ok {
		t.Fatalf("empty checkpoint must be a valid liveness write: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if len(got.State) != 0 {
		t.Fatalf("empty checkpoint state must read back empty: got %q", got.State)
	}
	if !got.LockedUntil.After(*before.LockedUntil) {
		t.Fatal("empty checkpoint must still renew the lease")
	}
}

func TestCheckpoint_AfterMarkComplete_NoOp(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_after_complete.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	_, _ = fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":1}`), testLease)
	if ok, err := fx.Runs.MarkComplete(ctx, r.ID, owner); err != nil || !ok {
		t.Fatalf("complete: ok=%v err=%v", ok, err)
	}

	ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":99}`), testLease)
	if err != nil || ok {
		t.Fatalf("checkpoint after complete: ok=%v err=%v want false,nil", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if string(got.State) != `{"step":1}` {
		t.Fatalf("late checkpoint must not overwrite: got %q", got.State)
	}
	if got.LockedUntil != nil {
		t.Fatal("late checkpoint must not re-arm the lease on a completed run")
	}
	// And it must not be claimable again.
	if next := claimAs(t, fx, newOwner("wB")); next != nil {
		t.Fatal("a completed run must not be resurrected into the claim queue")
	}
}

func TestCheckpoint_AfterMarkFailed_NoOp(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_after_failed.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	_, _ = fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":1}`), testLease)
	if ok, err := fx.Runs.MarkFailed(ctx, r.ID, owner, "dead"); err != nil || !ok {
		t.Fatalf("markfailed: ok=%v err=%v", ok, err)
	}

	ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"step":99}`), testLease)
	if err != nil || ok {
		t.Fatalf("checkpoint after failed: ok=%v err=%v want false,nil", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if string(got.State) != `{"step":1}` {
		t.Fatalf("failed run preserves last checkpoint: got %q", got.State)
	}
	if got.LockedUntil != nil {
		t.Fatal("late checkpoint must not re-arm a failed run")
	}
}

func TestCheckpoint_DoesNotMutateImmutableFields(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_immutable.db"))
	ctx := context.Background()
	r := enqueueRunInTenant(t, fx, "tnt-7")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, owner, []byte(`{"s":1}`), testLease); err != nil ||
		!ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if string(got.Payload) != `{}` {
		t.Fatalf("payload must be immutable: got %q", got.Payload)
	}
	if got.Kind != "agent" || got.TenantID != "tnt-7" || got.MaxRetries != 3 || got.Attempts != 0 {
		t.Fatalf("checkpoint mutated an immutable field: %+v", got)
	}
}

// Many concurrent checkpoints from the rightful owner must all succeed (SQLite
// serializes the autocommit writes; busy_timeout absorbs contention) with no
// lost-update error; the final state is one of the written values (last-writer-wins).
func TestCheckpoint_ConcurrentSameOwner_NoErrorNoLostUpdate(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "ck_concurrent.db"))
	ctx := context.Background()
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := fx.Runs.Checkpoint(
				ctx,
				r.ID,
				owner,
				[]byte(fmt.Sprintf(`{"w":%d}`, i)),
				testLease,
			)
			if err != nil {
				t.Errorf("concurrent checkpoint %d errored: %v", i, err)
			}
			if !ok {
				t.Errorf("concurrent checkpoint %d returned false (rightful owner)", i)
			}
		}(i)
	}
	wg.Wait()
	written := map[string]bool{}
	for i := 0; i < n; i++ {
		written[fmt.Sprintf(`{"w":%d}`, i)] = true
	}
	if got := mustFind(t, fx, r.ID); !written[string(got.State)] {
		t.Fatalf("final state %q must be exactly one of the concurrent writes", got.State)
	}
}
