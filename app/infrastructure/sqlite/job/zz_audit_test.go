package job_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/internal/testfx"
)

// TestRepository_ClaimDue_ReclaimsAfterLockExpiry closes audit claim
// infra-sched-job-40: an orphaned lock from a crashed worker must become
// re-claimable once locked_until expires, so a restarted worker resumes
// processing the very same persisted row from where it left off.
//
// Existing ClaimDue_SkipsLocked only proves a live lock stays locked. This
// drives the expiry transition: claim → (still locked, no-op) → wait past the
// lock → re-claim the SAME row via the real ClaimDue path.
//
// lockFor is 1s on purpose, not the sub-second value the audit note suggests:
// ClaimDue truncates the lock to whole seconds (int(lockFor.Seconds())), so
// anything below 1s becomes "+0 seconds" and the lock would expire
// immediately, making the "still locked" assertion timing-dependent. 1s is the
// smallest deterministic lock; the 1.5s sleep is a floor that only grows under
// load, so the reclaim cannot flake in the failing direction.
func TestRepository_ClaimDue_ReclaimsAfterLockExpiry(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "jobs_reclaim.db"))
	j := enqueueJob(t, fx, "orphaned")

	// First claim locks the row for ~1s and bumps attempts 0 → 1.
	first, err := fx.Jobs.ClaimDue(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if first == nil {
		t.Fatal("first claim should succeed")
	}
	if first.ID != j.ID {
		t.Fatalf("first claim id: got %q want %q", first.ID, j.ID)
	}
	if first.LockedUntil == nil {
		t.Fatal("locked_until must be set after claim (lock is active)")
	}
	if first.Attempts != 1 {
		t.Fatalf("attempts after first claim: got %d want 1", first.Attempts)
	}

	// While the lock is still live, a second claim must find nothing — proves
	// the lock is genuinely active, so the expiry transition below is meaningful.
	locked, err := fx.Jobs.ClaimDue(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("claim while locked: %v", err)
	}
	if locked != nil {
		t.Fatalf("job must not be claimable while lock is live, got %+v", locked)
	}

	// Simulate the crashed worker: the lock is orphaned and time marches past it.
	time.Sleep(1500 * time.Millisecond)

	// The expired lock must make the SAME persisted row re-claimable, and the
	// re-claim must go through the real ClaimDue path (attempts bumps 1 → 2).
	reclaimed, err := fx.Jobs.ClaimDue(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("reclaim after lock expiry: %v", err)
	}
	if reclaimed == nil {
		t.Fatal("orphaned job must be re-claimable after its lock expires")
	}
	if reclaimed.ID != j.ID {
		t.Fatalf("reclaimed id: got %q want %q (must be the same orphaned row)", reclaimed.ID, j.ID)
	}
	if reclaimed.Attempts != 2 {
		t.Fatalf(
			"attempts after reclaim: got %d want 2 (reclaim must bump attempts)",
			reclaimed.Attempts,
		)
	}
}
