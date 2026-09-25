package postgres_test

import (
	"testing"

	"gokick/app/infrastructure/database"
	"gokick/app/internal/testfx"
)

// The Postgres adapter's own tests run only when the run targets Postgres
// (make test-pg); a SQLite run skips them instead of failing for want of a server.
func TestMain(m *testing.M) { testfx.MainFor(m, database.DriverPostgres) }
