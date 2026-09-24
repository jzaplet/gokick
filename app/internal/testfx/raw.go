package testfx

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// Fixture writes that reach past the repositories — setting up a state the ports
// cannot produce (an expired lease, a deactivated user) or reading a table no
// port reads (audit_log). SQL here is the portable subset with ? placeholders,
// rebound for the active driver; the only dialect-specific piece is the DB-clock
// expression each backend supplies (backend.nowPlus). A test body therefore never
// carries SQL of its own dialect — the zz_nosqlite gate enforces that.

// Constraint names the integrity rule a failed write broke, classified from the
// driver's error code (never its message text — each database words it
// differently).
type Constraint string

const (
	NotNull    Constraint = "not null"
	Unique     Constraint = "unique" // a primary-key clash counts: Postgres reports it as one
	Check      Constraint = "check"
	ForeignKey Constraint = "foreign key"
)

// Violated reports which constraint err broke, or "" when err is not a
// constraint violation.
func (f *Fixture) Violated(err error) Constraint { return f.violation(err) }

// RawExec runs a portable statement past the repositories and returns its error
// unexamined — for tests that pin a schema constraint by attempting the write a
// repository would never issue.
func (f *Fixture) RawExec(query string, args ...any) (sql.Result, error) {
	return f.db.ExecContext(context.Background(), f.db.Rebind(query), args...)
}

// Count returns the number of rows in table matching where (empty = all rows).
func (f *Fixture) Count(t *testing.T, table, where string, args ...any) int {
	t.Helper()
	q := `SELECT COUNT(*) FROM ` + table
	if where != "" {
		q += ` WHERE ` + where
	}
	var n int
	if err := f.db.GetContext(context.Background(), &n, f.db.Rebind(q), args...); err != nil {
		t.Fatalf("testfx: count %s: %v", table, err)
	}
	return n
}

// AuditEntry is one audit_log row as stored.
type AuditEntry struct {
	Action      string  `db:"action"`
	ActorUserID *string `db:"actor_user_id"`
	ActorIP     *string `db:"actor_ip"`
	TargetType  *string `db:"target_type"`
	TargetID    *string `db:"target_id"`
	Metadata    []byte  `db:"metadata"`
}

// AuditEntry reads the audit_log row with the given id back — the log is
// append-only and has no read port.
func (f *Fixture) AuditEntry(t *testing.T, id string) AuditEntry {
	t.Helper()
	var e AuditEntry
	if err := f.db.GetContext(context.Background(), &e, f.db.Rebind(
		`SELECT action, actor_user_id, actor_ip, target_type, target_id, metadata
		   FROM audit_log WHERE id = ?`), id); err != nil {
		t.Fatalf("testfx: read audit entry %s: %v", id, err)
	}
	return e
}

// SetUserActive flips a user's active flag directly (the account-disabled state
// the auth tests start from).
func (f *Fixture) SetUserActive(t *testing.T, userID string, active bool) {
	t.Helper()
	f.execOne(t, "set user active", `UPDATE users SET active = ? WHERE id = ?`, active, userID)
}

// SetUserLockedUntil stamps a brute-force lock expiry onto a user.
func (f *Fixture) SetUserLockedUntil(t *testing.T, userID string, until time.Time) {
	t.Helper()
	f.execOne(t, "set user lock", `UPDATE users SET locked_until = ? WHERE id = ?`, until, userID)
}

// ForceExpireLease backdates a run's lease by an hour, making it reclaimable at
// once — deterministic, where a short lease plus a sleep would be racy.
func (f *Fixture) ForceExpireLease(t *testing.T, runID string) {
	t.Helper()
	f.SetLeaseFromNow(t, runID, -time.Hour)
}

// SetLeaseFromNow sets a run's lease expiry to the DATABASE clock plus d (d may
// be negative) — the clock the claim compares against, so sub-second offsets are
// exact.
func (f *Fixture) SetLeaseFromNow(t *testing.T, runID string, d time.Duration) {
	t.Helper()
	f.execOne(t, "set lease",
		`UPDATE runs SET locked_until = `+f.nowPlus+` WHERE id = ?`, d.Seconds(), runID)
}

// StealLease hands a run's lease to newOwner with an hour to run — another worker
// reclaiming it in ONE write, so a live incumbent's heartbeat cannot interleave
// and the steal is deterministic. The incumbent's next owner-checked write then
// matches no row.
func (f *Fixture) StealLease(t *testing.T, runID, newOwner string) {
	t.Helper()
	f.execOne(t, "steal lease",
		`UPDATE runs SET locked_by = ?, locked_until = `+f.nowPlus+` WHERE id = ?`,
		newOwner, time.Hour.Seconds(), runID)
}

// MakeRunDue moves a run's run_at a second into the past — clearing a retry
// backoff so the run is claimable now.
func (f *Fixture) MakeRunDue(t *testing.T, runID string) {
	t.Helper()
	f.execOne(t, "make run due",
		`UPDATE runs SET run_at = `+f.nowPlus+` WHERE id = ?`, (-time.Second).Seconds(), runID)
}

// ForceRunCompleted / ForceRunFailed stamp a terminal column directly,
// independent of the finalizers — for tests asserting what the queue does with a
// terminal row, not how a run gets there (MarkRunCompleted takes the real path).
func (f *Fixture) ForceRunCompleted(t *testing.T, runID string) {
	t.Helper()
	f.execOne(t, "force completed",
		`UPDATE runs SET completed_at = `+f.nowPlus+` WHERE id = ?`, 0.0, runID)
}

func (f *Fixture) ForceRunFailed(t *testing.T, runID string) {
	t.Helper()
	f.execOne(t, "force failed",
		`UPDATE runs SET failed_at = `+f.nowPlus+` WHERE id = ?`, 0.0, runID)
}

// SetRunReclaims / SetRunParks preset a run's crash-reclaim and registry-skew
// counters (a poison run already past its cap).
func (f *Fixture) SetRunReclaims(t *testing.T, runID string, n int) {
	t.Helper()
	f.execOne(t, "set reclaims", `UPDATE runs SET reclaims = ? WHERE id = ?`, n, runID)
}

func (f *Fixture) SetRunParks(t *testing.T, runID string, n int) {
	t.Helper()
	f.execOne(t, "set parks", `UPDATE runs SET parks = ? WHERE id = ?`, n, runID)
}

// setTenantPlan backs SeedTenantWithPlan.
func (f *Fixture) setTenantPlan(t *testing.T, tenantID, plan string) {
	t.Helper()
	f.execOne(t, "set tenant plan", `UPDATE tenants SET plan = ? WHERE id = ?`, plan, tenantID)
}

// execOne runs a fixture write and requires it to touch exactly one row: a
// fixture that silently matched nothing would leave the test asserting against a
// state it never set up.
func (f *Fixture) execOne(t *testing.T, what, query string, args ...any) {
	t.Helper()
	res, err := f.RawExec(query, args...)
	if err != nil {
		t.Fatalf("testfx: %s: %v", what, err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("testfx: %s: affected %d rows, want 1 (err %v)", what, n, err)
	}
}
