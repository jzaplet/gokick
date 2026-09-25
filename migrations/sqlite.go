//go:build !nosqlite

package migrations

import "embed"

//go:embed sqlite/*.sql
var sqliteFS embed.FS

// SQLite is the SQLite dialect's migration set, rooted so goose sees the files
// at "." — what goose.NewProvider expects.
var SQLite = mustSub(sqliteFS, "sqlite")
