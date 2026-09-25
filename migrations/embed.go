package migrations

import (
	"embed"
	"io/fs"
)

// One directory per dialect, each embedded by its own file: sqlite.go carries the
// nosqlite build tag, so a -tags nosqlite binary embeds no SQLite SQL at all. The
// SQL differs per engine, but the VERSIONS must stay in lock-step across dialects
// so any deployment reaches the same logical schema whichever adapter it runs
// (see /gk-migrations; app/zz_migrations_test.go enforces it).

//go:embed postgres/*.sql
var postgresFS embed.FS

// Postgres is the Postgres dialect's migration set, rooted so goose sees the
// files at "." — what goose.NewProvider expects.
var Postgres = mustSub(postgresFS, "postgres")

// mustSub roots fsys at dir. fs.Sub only fails on an invalid path, which here is
// a compile-time constant — a failure is a programming error caught by any test
// that boots the migrator.
func mustSub(fsys embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("migrations: " + err.Error())
	}
	return sub
}
