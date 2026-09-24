//go:build !nosqlite

package sqlite_test

import (
	"testing"

	"gokick/app/infrastructure/database"
	"gokick/app/internal/testfx"
)

// The SQLite adapter's own tests run only when the run targets SQLite: a run
// with APP_DB_DRIVER=postgres must not touch SQLite anywhere.
func TestMain(m *testing.M) { testfx.MainFor(m, database.DriverSQLite) }
