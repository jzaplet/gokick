package postgres

import (
	"strconv"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"
)

// Time discipline — the invariant the zz_pgtime gate enforces: a time the
// database stamps comes from statement_timestamp(), never now() /
// CURRENT_TIMESTAMP / transaction_timestamp(). Those are the START of the
// transaction, so inside a longer transaction every stamp and every lease would
// stand still at its first statement. statement_timestamp() is the start of the
// statement — the same clock SQLite's 'now' is.

// NowExpr is the database clock (completed_at, updated_at, used_at, …).
const NowExpr = `statement_timestamp()`

// NowPlus is the database clock plus the number of seconds (a float, so a
// sub-second lease is not truncated) bound to the placeholder param:
// NowPlus("$2") for a lease of $2 seconds.
func NowPlus(param string) string {
	return `statement_timestamp() + make_interval(secs => ` + param + `)`
}

// Args collects the arguments of a statement built piece by piece — the grids'
// optional filters — and hands out their $n placeholders in order. Preload the
// fixed arguments, so the statement's head stays one literal with its own $n
// (the tenant conformance gate reads SQL literal by literal):
//
//	a := postgres.Args{tenantID}
//	q := `SELECT * FROM users WHERE tenant_id = $1` + filters(&a) // filters: a.Add(v) → "$2", …
//	db.SelectContext(ctx, &rows, q, a...)
type Args []any

// Add appends v and returns its placeholder.
func (a *Args) Add(v any) string {
	*a = append(*a, v)
	return "$" + strconv.Itoa(len(*a))
}

// ILikeContains is a case-insensitive literal-substring match of column against
// s: ILIKE under the Czech sort collation (the ICU collation folds case for Č, Ř,
// Ž … too, where the database's own ctype might fold ASCII only), with s escaped
// by database.LikeContains so a % or _ the user typed is searched for, not
// treated as a wildcard. It is the twin of the SQLite adapter's Unicode LIKE —
// the golden corpus test checks both find the same rows.
func ILikeContains(column string, a *Args, s string) string {
	return column + database.CollateSort + ` ILIKE ` + a.Add(
		database.LikeContains(s),
	) + database.LikeEscape
}

// NullsSmallest is the NULLS clause that orders NULL the way SQLite does — as the
// smallest value: first ascending, last descending. Postgres treats NULL as the
// largest. Append it to a sort on a nullable column, and only there: on a column
// that is never NULL it changes no order, but it keeps an index that stores the
// default null order from serving the sort.
func NullsSmallest(dir shared.SortDirection) string {
	if dir == shared.SortDesc {
		return " NULLS LAST"
	}
	return " NULLS FIRST"
}

// LockInIDOrder closes a subquery that selects the rows a write is about to
// change: it locks them in id order first. Two writes over overlapping rows then
// queue up on the first row they share, instead of each locking part of the set
// in its own scan order and deadlocking on the rest. (A deadlock that happens
// anyway is retried by the bus — Manager.IsRetryable — but costs a second
// attempt.)
//
//	`UPDATE users SET … WHERE id IN (SELECT id FROM users WHERE …` + postgres.LockInIDOrder + `)`
const LockInIDOrder = ` ORDER BY id FOR UPDATE`
