package sqlite

import "database/sql"

// Time discipline for DATETIME (TEXT) columns — the invariant the zz_sqltime
// gate enforces:
//
// Datetime columns hold TWO text encodings. Rows written from Go carry RFC3339
// ("2026-07-14T10:00:00.123Z" — 'T' separator, 'Z' suffix); rows stamped by the
// DB clock (NowExpr / LeaseExpr) carry SQLite's own format
// ("2026-07-14 10:00:00.123" — space separator, no timezone). The two do NOT
// compare as raw strings ('T' sorts above ' '), so a bare-column relational
// comparison or ORDER BY silently misorders mixed rows. Compare and order
// datetime columns ONLY via julianday(col) — never as raw text, and never via
// datetime(col) either, so the repo keeps exactly one comparison idiom
// (millisecond-precise and encoding-agnostic).

// NowExpr writes the DB clock at millisecond precision (completed_at / failed_at /
// cancelled_at / updated_at and other DB-sourced timestamps). It is the SQL-side
// companion to MsPrecisionUTC: shared by the durable-queue repos so the write-clock
// format cannot drift between queues.
const NowExpr = `strftime('%Y-%m-%d %H:%M:%f', 'now')`

// LeaseExpr computes locked_until = now + lease entirely in SQLite, sub-second
// precise: julianday('now') is a double (days), + lease_seconds/86400 adds the lease
// as a fraction of a day, strftime formats it back at ms precision. The bound
// parameter is the lease in seconds (a float64). Unlike a '+%d seconds' modifier it
// does NOT truncate a sub-second lease to +0s. Shared so the lease arithmetic lives
// in one place for every fenced queue.
const LeaseExpr = `strftime('%Y-%m-%d %H:%M:%f', julianday('now') + ? / 86400.0)`

// NotTerminalClause is the SQL predicate for a run that has NOT reached a terminal
// state — none of the three terminal timestamps is set. It is the single source of
// the terminal-state rule shared by every run query (claim, renew, checkpoint,
// finalizers) so the derivation can't drift across the ~9 call sites, exactly like
// NowExpr/LeaseExpr. Bare column names (no table alias): it slots into a WHERE on
// the runs table. The domain mirror for the read path is run.Run.IsTerminal.
const NotTerminalClause = `completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`

// RowsAffectedBool turns a conditional UPDATE result into the exactly-one-row
// bool: (true, nil) iff exactly one row was affected, (false, nil) when zero rows
// matched (contention lost / terminal — NOT an error), (false, err) on a real
// write failure. This is the contract owner-fencing rests on — a finalizer that
// ignores sql.Result would silently report success on a zero-row stale write —
// and the same contract the refresh-token rotation CAS needs. Shared so every
// exactly-one-row consumer (the run finalizers and the token MarkUsed CAS today,
// a Postgres repo later) reuses one implementation and can't hand-roll a divergent
// copy that re-introduces the silent zero-row trap.
func RowsAffectedBool(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
