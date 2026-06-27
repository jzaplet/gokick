package sqlite

import "database/sql"

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

// RowsAffectedBool turns an owner-checked UPDATE result into the fencing bool:
// (true, nil) iff exactly one row was affected, (false, nil) when zero rows matched
// (ownership lost / terminal — NOT an error), (false, err) on a real write failure.
// This is the contract owner-fencing rests on — a finalizer that ignores sql.Result
// would silently report success on a zero-row stale write. Shared so every fenced
// repo (the SQLite run repo today, a Postgres one later) reuses one implementation.
func RowsAffectedBool(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
