//go:build !nosqlite

package sqlite

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
// companion to database.MsPrecisionUTC: shared by the durable-queue repos so the write-clock
// format cannot drift between queues.
const NowExpr = `strftime('%Y-%m-%d %H:%M:%f', 'now')`

// LeaseExpr computes locked_until = now + lease entirely in SQLite, sub-second
// precise: julianday('now') is a double (days), + lease_seconds/86400 adds the lease
// as a fraction of a day, strftime formats it back at ms precision. The bound
// parameter is the lease in seconds (a float64). Unlike a '+%d seconds' modifier it
// does NOT truncate a sub-second lease to +0s. Shared so the lease arithmetic lives
// in one place for every fenced queue.
const LeaseExpr = `strftime('%Y-%m-%d %H:%M:%f', julianday('now') + ? / 86400.0)`
