package database

import "fmt"

// Driver names a database adapter. APP_DB_DRIVER picks one at startup, and the
// test harness (internal/testfx) reads the same variable, so a test run exercises
// exactly the adapter a deployment configured the same way would use.
type Driver string

const (
	// DriverSQLite is the default: one file, no server to run.
	DriverSQLite Driver = "sqlite"
	// DriverPostgres is the Postgres adapter (see the postgres-adapter-plan doc).
	DriverPostgres Driver = "postgres"
)

// ParseDriver accepts exactly the known driver names. Anything else is an error,
// so a typo in APP_DB_DRIVER fails fast instead of quietly running the app (or a
// test suite) on the default adapter.
func ParseDriver(s string) (Driver, error) {
	switch d := Driver(s); d {
	case DriverSQLite, DriverPostgres:
		return d, nil
	default:
		return "", fmt.Errorf("unknown database driver %q: want %q or %q",
			s, DriverSQLite, DriverPostgres)
	}
}
