package run_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"

	"github.com/google/uuid"
)

// testLease is a comfortably-whole-second lease — jobs truncates int(sec) so a
// sub-second lease would expire instantly; runs is sub-second safe but the shared
// helpers use a whole-second lease for clarity.
const testLease = time.Minute

// newOwner returns a per-claim owner nonce (workerID + uuid), as the port
// requires — a stable per-worker id could not fence a worker against its own
// earlier abandoned goroutine.
func newOwner(prefix string) string { return prefix + "-" + uuid.NewString() }

func enqueueRun(t *testing.T, fx *testfx.Fixture, kind string) *run.Run {
	t.Helper()
	r := run.NewRun(kind, []byte(`{}`), 3)
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return r
}

func enqueueRunAt(t *testing.T, fx *testfx.Fixture, kind string, runAt time.Time) *run.Run {
	t.Helper()
	r := run.NewRun(kind, []byte(`{}`), 3)
	r.RunAt = runAt
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return r
}

func claimAs(t *testing.T, fx *testfx.Fixture, owner string) *run.Run {
	t.Helper()
	r, err := fx.Runs.ClaimDue(context.Background(), owner, testLease)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return r
}

// forceExpire backdates a run's lease via a raw write so it is reclaimable —
// deterministic, no sleeping (a sub-second lease + sleep would be racy).
func forceExpire(t *testing.T, fx *testfx.Fixture, id string) {
	t.Helper()
	res, err := fx.DB.DB().ExecContext(context.Background(),
		`UPDATE runs SET locked_until = strftime('%Y-%m-%d %H:%M:%f','now','-1 hour') WHERE id = ?`, id)
	if err != nil {
		t.Fatalf("force-expire: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("force-expire affected %d rows, want 1", n)
	}
}

func mustFind(t *testing.T, fx *testfx.Fixture, id string) *run.Run {
	t.Helper()
	got, err := fx.Runs.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatalf("run %s not found", id)
	}
	return got
}

// ─── Enqueue ────────────────────────────────────────────────────────────────

func TestEnqueue_PersistsAndRoundTrips(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "enq.db"))
	r := run.NewRun("agent:summarize", []byte(`{"input":"x"}`), 3)
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got := mustFind(t, fx, r.ID)
	if got.Kind != "agent:summarize" {
		t.Fatalf("kind: got %q", got.Kind)
	}
	if string(got.Payload) != `{"input":"x"}` {
		t.Fatalf("payload: got %q", got.Payload)
	}
	if got.Attempts != 0 || got.Reclaims != 0 {
		t.Fatalf("attempts/reclaims: got %d/%d want 0/0", got.Attempts, got.Reclaims)
	}
	if got.MaxRetries != 3 {
		t.Fatalf("max_retries: got %d want 3", got.MaxRetries)
	}
	// The driver surfaces both a NULL column and an empty blob as a zero-length
	// slice, so "never checkpointed" is observable as len(State)==0, not nil.
	if len(got.State) != 0 {
		t.Fatalf("state must be empty until first checkpoint, got %q", got.State)
	}
	if got.LockedBy != nil || got.LockedUntil != nil {
		t.Fatal("fresh run must be unlocked")
	}
	if got.TenantID != shared.DefaultTenantID {
		t.Fatalf("tenant: got %q want default %q", got.TenantID, shared.DefaultTenantID)
	}
}

func TestEnqueue_InTxRollback_LeavesNoRow(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "enq_rollback.db"))
	txCtx, err := fx.DB.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	r := run.NewRun("tx", []byte(`{}`), 0)
	if err := fx.Runs.Enqueue(txCtx, r); err != nil {
		t.Fatalf("enqueue in tx: %v", err)
	}
	if err := fx.DB.Rollback(txCtx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	got, err := fx.Runs.FindByID(context.Background(), r.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nil {
		t.Fatal("Enqueue must honor Conn(ctx): a rolled-back tx leaves no row")
	}
}

func TestEnqueue_InTxCommit_Persists(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "enq_commit.db"))
	txCtx, err := fx.DB.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	r := run.NewRun("tx", []byte(`{}`), 0)
	if err := fx.Runs.Enqueue(txCtx, r); err != nil {
		t.Fatalf("enqueue in tx: %v", err)
	}
	if err := fx.DB.Commit(txCtx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if got := mustFind(t, fx, r.ID); got.Kind != "tx" {
		t.Fatalf("committed enqueue not visible: %+v", got)
	}
}

func TestEnqueue_DuplicateIDErrors(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "enq_dup.db"))
	r := run.NewRun("dup", []byte(`{}`), 0)
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := fx.Runs.Enqueue(context.Background(), r); err == nil {
		t.Fatal("duplicate primary key must error (no silent upsert)")
	}
}

// ─── ClaimDue ───────────────────────────────────────────────────────────────

func TestClaimDue_EmptyReturnsNil(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_empty.db"))
	got, err := fx.Runs.ClaimDue(context.Background(), newOwner("w"), testLease)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got != nil {
		t.Fatalf("empty queue must return nil, got %+v", got)
	}
}

// CORRECTED vs the auto-generated matrix: a claim does NOT bump attempts (that
// would burn a long run's retry budget on crash-reclaims). Claim stamps the owner
// + lease; attempts stays 0 until a logic Reschedule; reclaims stays 0 on a first
// claim of a fresh row.
func TestClaimDue_StampsOwnerAndLease_DoesNotBumpAttempts(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_one.db"))
	enqueueRun(t, fx, "only")
	owner := newOwner("wA")

	got := claimAs(t, fx, owner)
	if got == nil {
		t.Fatal("expected a claim")
	}
	if got.Attempts != 0 {
		t.Fatalf("attempts: got %d want 0 (claim must NOT bump attempts)", got.Attempts)
	}
	if got.Reclaims != 0 {
		t.Fatalf(
			"reclaims: got %d want 0 (first claim of a fresh run is not a reclaim)",
			got.Reclaims,
		)
	}
	if got.LockedBy == nil || *got.LockedBy != owner {
		t.Fatalf("locked_by: got %v want %q", got.LockedBy, owner)
	}
	if got.LockedUntil == nil {
		t.Fatal("locked_until must be set after claim")
	}
	now := time.Now()
	if got.LockedUntil.Before(now) || got.LockedUntil.After(now.Add(testLease+2*time.Second)) {
		t.Fatalf("locked_until %v not within (now, now+lease+slack]", got.LockedUntil)
	}
}

func TestClaimDue_PicksOldestRunAtFirst(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_order.db"))
	now := time.Now()
	enqueueRunAt(t, fx, "second", now.Add(-1*time.Second))
	enqueueRunAt(t, fx, "first", now.Add(-2*time.Second))

	c1 := claimAs(t, fx, newOwner("w1"))
	if c1 == nil || c1.Kind != "first" {
		t.Fatalf("first claim: got %v want kind=first (ORDER BY run_at)", c1)
	}
	c2 := claimAs(t, fx, newOwner("w2"))
	if c2 == nil || c2.Kind != "second" {
		t.Fatalf("second claim: got %v want kind=second", c2)
	}
	if c3 := claimAs(t, fx, newOwner("w3")); c3 != nil {
		t.Fatalf("third claim must be nil (all claimed), got %v", c3)
	}
}

func TestClaimDue_SkipsLockedRun(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_locked.db"))
	enqueueRun(t, fx, "only")
	if first := claimAs(t, fx, newOwner("wA")); first == nil {
		t.Fatal("first claim should succeed")
	}
	if second := claimAs(t, fx, newOwner("wB")); second != nil {
		t.Fatal("a run with a live lease must not be claimable by another owner")
	}
}

func TestClaimDue_SkipsFutureRunAt(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_future.db"))
	enqueueRunAt(t, fx, "later", time.Now().Add(time.Hour))
	if got := claimAs(t, fx, newOwner("w")); got != nil {
		t.Fatalf("future run_at must not be claimable, got %v", got)
	}
}

func TestClaimDue_NeverReturnsCompletedOrFailed(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_terminal.db"))
	rc := enqueueRun(t, fx, "done")
	rf := enqueueRun(t, fx, "dead")
	ctx := context.Background()
	// Raw-set terminal columns (independent of the finalize impl).
	if _, err := fx.DB.DB().ExecContext(ctx,
		`UPDATE runs SET completed_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE id = ?`, rc.ID); err != nil {
		t.Fatalf("set completed: %v", err)
	}
	if _, err := fx.DB.DB().ExecContext(ctx,
		`UPDATE runs SET failed_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE id = ?`, rf.ID); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if got := claimAs(t, fx, newOwner("w")); got != nil {
		t.Fatalf("completed/failed runs must never be claimed, got kind=%q", got.Kind)
	}
}

// Reclaiming an EXPIRED lease flips the owner token (the fencing precondition) and
// bumps reclaims — but NOT attempts.
func TestClaimDue_ReclaimsExpiredLease_FlipsOwner_BumpsReclaimsNotAttempts(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_reclaim.db"))
	r := enqueueRun(t, fx, "agent")
	ownerA, ownerB := newOwner("wA"), newOwner("wB")

	a := claimAs(t, fx, ownerA)
	if a.Attempts != 0 || a.Reclaims != 0 {
		t.Fatalf("after first claim: attempts/reclaims %d/%d want 0/0", a.Attempts, a.Reclaims)
	}
	forceExpire(t, fx, r.ID)

	b := claimAs(t, fx, ownerB)
	if b == nil || b.ID != r.ID {
		t.Fatalf("reclaim: got %v want same id", b)
	}
	if b.LockedBy == nil || *b.LockedBy != ownerB {
		t.Fatalf("reclaim must flip locked_by to B: got %v", b.LockedBy)
	}
	if b.Reclaims != 1 {
		t.Fatalf("reclaims: got %d want 1 (expired-lease reclaim bumps reclaims)", b.Reclaims)
	}
	if b.Attempts != 0 {
		t.Fatalf("attempts: got %d want 0 (a crash-reclaim is NOT a logic retry)", b.Attempts)
	}
}

func TestClaimDue_FreshlyEnqueuedNow_ImmediatelyClaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_now.db"))
	// NewRun sets RunAt=time.Now() at µs precision; msPrecisionUTC + julianday
	// must keep it <= 'now'. A regression here (strftime '%f' rounding) makes
	// this flaky.
	enqueueRun(t, fx, "now")
	if got := claimAs(t, fx, newOwner("w")); got == nil {
		t.Fatal("a run enqueued at now() must be immediately claimable")
	}
}

func TestClaimDue_SubSecondDelay_NotClaimedUntilDue(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_delay.db"))
	enqueueRunAt(t, fx, "soon", time.Now().Add(800*time.Millisecond))
	if early := claimAs(t, fx, newOwner("w")); early != nil {
		t.Fatal("a sub-second-future run must not be claimed early (julianday precision)")
	}
	time.Sleep(900 * time.Millisecond)
	if due := claimAs(t, fx, newOwner("w")); due == nil {
		t.Fatal("run must be claimable once run_at passes")
	}
}

func TestClaimDue_LeaseZeroOrNegative_Rejected(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "claim_lease0.db"))
	r := enqueueRun(t, fx, "x")
	for _, lease := range []time.Duration{0, -time.Second} {
		got, err := fx.Runs.ClaimDue(context.Background(), newOwner("w"), lease)
		if err == nil {
			t.Fatalf("lease %s must be rejected", lease)
		}
		if got != nil {
			t.Fatalf("lease %s: got %v want nil", lease, got)
		}
	}
	// Row must be unmutated by a rejected claim.
	if got := mustFind(t, fx, r.ID); got.LockedBy != nil || got.LockedUntil != nil {
		t.Fatal("a rejected claim must not mutate the row")
	}
}

// ─── Lease lifecycle ──────────────────────────────────────────────────────────

func TestRenewLease_ExtendsAbsolute_KeepsUnclaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lease_renew.db"))
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	// Claim with a LONGER lease, renew with a shorter one: an absolute reset to
	// now+lease moves locked_until FORWARD in time relative to the read clock but
	// the key check is it lands within now+shortLease (not now+longLease).
	claimed, err := fx.Runs.ClaimDue(context.Background(), owner, 2*time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v / %v", claimed, err)
	}
	ok, err := fx.Runs.RenewLease(context.Background(), r.ID, owner, 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("renew by owner must succeed: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	now := time.Now()
	if got.LockedUntil.After(now.Add(30*time.Second + 2*time.Second)) {
		t.Fatalf(
			"renew must set locked_until = now+lease (absolute), got %v (now+30s expected)",
			got.LockedUntil,
		)
	}
	// Still unclaimable by another owner.
	if other := claimAs(t, fx, newOwner("wB")); other != nil {
		t.Fatal("a renewed lease must keep the run unclaimable by another owner")
	}
}

func TestClaimDue_ExpiredByOneMs_Claimable_FutureNot(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lease_precision.db"))
	ctx := context.Background()

	// 1ms in the PAST → claimable (julianday sub-ms aware).
	past := enqueueRun(t, fx, "past")
	claimAs(t, fx, newOwner("wA")) // lock it
	if _, err := fx.DB.DB().ExecContext(ctx,
		`UPDATE runs SET locked_until = strftime('%Y-%m-%d %H:%M:%f', julianday('now') - 1.0/86400000.0) WHERE id = ?`, past.ID); err != nil {
		t.Fatalf("set past: %v", err)
	}
	if got := claimAs(t, fx, newOwner("wB")); got == nil {
		t.Fatal("a lease expired by 1ms must be claimable")
	}

	// 300ms in the FUTURE → NOT claimable. THIS is the precision discriminator:
	// a datetime('now') (second-truncated) comparison would wrongly hand it out.
	fut := enqueueRun(t, fx, "future")
	claimAs(t, fx, newOwner("wC"))
	if _, err := fx.DB.DB().ExecContext(ctx,
		`UPDATE runs SET locked_until = strftime('%Y-%m-%d %H:%M:%f', julianday('now') + 300.0/86400000.0) WHERE id = ?`, fut.ID); err != nil {
		t.Fatalf("set future: %v", err)
	}
	if got, _ := fx.Runs.ClaimDue(ctx, newOwner("wD"), testLease); got != nil && got.ID == fut.ID {
		t.Fatal(
			"a lease 300ms in the future must NOT be claimable (julianday, not second-truncation)",
		)
	}
}

func TestRenewLease_LeaseZeroOrNegative_Rejected_RunStaysHeld(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lease_renew0.db"))
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	before := mustFind(t, fx, r.ID)

	for _, lease := range []time.Duration{0, -time.Second} {
		ok, err := fx.Runs.RenewLease(context.Background(), r.ID, owner, lease)
		if err == nil || ok {
			t.Fatalf("renew lease %s must be rejected: ok=%v err=%v", lease, ok, err)
		}
	}
	after := mustFind(t, fx, r.ID)
	if !after.LockedUntil.Equal(*before.LockedUntil) {
		t.Fatal("a rejected renew must not collapse/change the live lease")
	}
}

func TestClaimDue_SubSecondLease_NotInstantlyReclaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lease_subsec.db"))
	enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	// 800ms lease must NOT truncate to +0s (the jobs int-seconds trap): the run
	// must stay held for ~800ms, not be instantly reclaimable.
	claimed, err := fx.Runs.ClaimDue(context.Background(), owner, 800*time.Millisecond)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v / %v", claimed, err)
	}
	if other, _ := fx.Runs.ClaimDue(context.Background(), newOwner("wB"), testLease); other != nil {
		t.Fatal(
			"an 800ms lease must hold the run (not truncate to +0s and be instantly reclaimable)",
		)
	}
}

func TestCheckpoint_RenewsLease_PersistsState_KeepsUnclaimable(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lease_ckpt.db"))
	r := enqueueRun(t, fx, "agent")
	owner := newOwner("wA")
	claimAs(t, fx, owner)
	before := mustFind(t, fx, r.ID)

	time.Sleep(5 * time.Millisecond)
	ok, err := fx.Runs.Checkpoint(
		context.Background(),
		r.ID,
		owner,
		[]byte(`{"step":1}`),
		testLease,
	)
	if err != nil || !ok {
		t.Fatalf("checkpoint by owner must succeed: ok=%v err=%v", ok, err)
	}
	got := mustFind(t, fx, r.ID)
	if string(got.State) != `{"step":1}` {
		t.Fatalf("state: got %q want {\"step\":1}", got.State)
	}
	if !got.LockedUntil.After(*before.LockedUntil) {
		t.Fatal("a successful checkpoint must renew (advance) the lease")
	}
	if other := claimAs(t, fx, newOwner("wB")); other != nil {
		t.Fatal("a checkpoint-renewed lease must keep the run unclaimable")
	}
}
