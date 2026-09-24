package migrations

import (
	"embed"
	"io/fs"
)

// One directory per dialect. The SQL differs per engine, but the VERSIONS must
// stay in lock-step across dialects so any deployment reaches the same logical
// schema whichever adapter it runs (see /gk-migrations).
//
//go:embed sqlite/*.sql
var all embed.FS

// SQLite is the SQLite dialect's migration set, rooted so goose sees the files
// at "." — what goose.NewProvider expects.
var SQLite = mustSub("sqlite")

// mustSub roots fsys at dir. fs.Sub only fails on an invalid path, which here is
// a compile-time constant — a failure is a programming error caught by any test
// that boots the migrator.
func mustSub(dir string) fs.FS {
	sub, err := fs.Sub(all, dir)
	if err != nil {
		panic("migrations: " + err.Error())
	}
	return sub
}
