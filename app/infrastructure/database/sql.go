package database

import (
	"database/sql"
	"strings"
	"time"
)

// SQL building blocks every adapter's repositories share. Each is portable across
// the dialects gokick runs on, so the rule it encodes is written once.

// CollateSort is the ORDER BY suffix for user-facing text sorts:
// `ORDER BY nickname` + CollateSort. Sort whitelists append it to text columns
// only — ids, numbers and timestamps keep their natural order.
const CollateSort = " COLLATE " + SortCollation

// LikeEscape declares the escape character LikeContains uses. Append it to every
// LIKE / ILIKE that takes a LikeContains argument: `nickname LIKE ?` + LikeEscape.
const LikeEscape = ` ESCAPE '\'`

// likeEscaper escapes LIKE's wildcards and the escape character itself.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikeContains is the LIKE argument that matches s as a literal substring: a %
// or _ the user typed into a search box is searched for, not treated as a
// wildcard. Case-insensitivity is the adapter's part (SQLite's Unicode LIKE,
// Postgres ILIKE under the ICU sort collation).
func LikeContains(s string) string {
	return "%" + likeEscaper.Replace(s) + "%"
}

// MsPrecisionUTC normalizes a Go time.Time to UTC + millisecond precision before
// it is written. Every adapter stores at least millisecond precision (Postgres
// keeps microseconds, SQLite the text the value is written as), so a value
// normalized here reads back exactly equal on each of them. On SQLite it also
// dodges a clock race: ncruces' WASM 'now' ticks at ~1 ms and trails Go's
// time.Now() by up to ~1 ms, so a time written at µs precision could lose the
// run_at <= now comparison and a freshly-enqueued row be missed.
func MsPrecisionUTC(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}

// NotTerminalClause is the SQL predicate for a run that has NOT reached a terminal
// state — none of the three terminal timestamps is set. It is the single source of
// the terminal-state rule shared by every run query (claim, renew, checkpoint,
// finalizers, the tenant delete's "owns nothing live") on every adapter, so the
// derivation can't drift across the call sites. Bare column names (no table
// alias): it slots into a WHERE on the runs table. The domain mirror for the read
// path is run.Run.IsTerminal.
const NotTerminalClause = `completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`

// RowsAffectedBool turns a conditional UPDATE result into the exactly-one-row
// bool: (true, nil) iff exactly one row was affected, (false, nil) when zero rows
// matched (contention lost / terminal — NOT an error), (false, err) on a real
// write failure. This is the contract owner-fencing rests on — a finalizer that
// ignores sql.Result would silently report success on a zero-row stale write —
// and the same contract the refresh-token rotation CAS needs. Shared so every
// exactly-one-row consumer (the run finalizers and the token MarkUsed CAS, on
// every adapter) reuses one implementation and can't hand-roll a divergent copy
// that re-introduces the silent zero-row trap.
func RowsAffectedBool(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
