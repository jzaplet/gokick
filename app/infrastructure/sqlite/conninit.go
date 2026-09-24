//go:build !nosqlite

package sqlite

import (
	"errors"

	"gokick/app/infrastructure/database"

	"github.com/google/uuid"
	"github.com/ncruces/go-sqlite3"
	sqliteunicode "github.com/ncruces/go-sqlite3/ext/unicode"
)

// registerConnFuncs is the driver's per-connection init callback. It runs on
// EVERY connection the pool opens — a function or collation registered on one
// connection only would be missing on the next one the pool hands out.
//
// It gives SQLite what the rest of the app (and a future Postgres adapter) relies
// on being identical across backends:
//   - Unicode-aware LIKE / upper / lower: search ignores case for Č, Ř, Ž … too,
//     the way Postgres ILIKE does (SQLite's built-in LIKE folds ASCII only);
//   - the database.SortCollation collation for Czech alphabetical ORDER BY;
//   - the uuidv7() SQL function, the name Postgres 18 ships natively, so a
//     hand-written seed mints ids the same way on both backends.
func registerConnFuncs(c *sqlite3.Conn) error {
	return errors.Join(
		sqliteunicode.Register(c),
		sqliteunicode.RegisterCollation(c, database.SortLocale, database.SortCollation),
		c.CreateFunction("uuidv7", 0, sqlite3.INNOCUOUS, uuidV7SQL),
	)
}

// uuidV7SQL implements the SQL function uuidv7(): a time-ordered UUIDv7, the id
// format every table uses.
func uuidV7SQL(ctx sqlite3.Context, _ ...sqlite3.Value) {
	id, err := uuid.NewV7()
	if err != nil {
		ctx.ResultError(err)
		return
	}
	ctx.ResultText(id.String())
}

// CollateSort is the ORDER BY suffix for user-facing text sorts:
// `ORDER BY nickname` + CollateSort. Sort whitelists append it to text columns
// only — ids, numbers and timestamps keep their natural order.
const CollateSort = " COLLATE " + database.SortCollation
