//go:build nosqlite

package testfx

import (
	"log/slog"
	"testing"

	"gokick/app/infrastructure/config"
)

// openSQLite in a -tags nosqlite build: the adapter is not linked, so a test that
// still asks for SQLite fails loudly instead of running on some other database.
func openSQLite(t *testing.T, _ *config.Config, _ *slog.Logger) backend {
	t.Helper()
	t.Fatal("testfx: APP_DB_DRIVER=sqlite, but this test binary was built with -tags nosqlite")
	return backend{}
}
