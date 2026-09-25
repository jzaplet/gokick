package testfx

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"gokick/app/infrastructure/database"
)

// activeDriver parses APP_DB_DRIVER once per test binary. It reads the process
// environment only — never .env — so a developer's local app config cannot flip
// which database the suite runs against; `make test` and `make test-pg` set it.
var activeDriver = sync.OnceValues(func() (database.Driver, error) {
	v := os.Getenv("APP_DB_DRIVER")
	if v == "" {
		return database.DriverSQLite, nil
	}
	return database.ParseDriver(v)
})

// ActiveDriver is the database adapter this test run targets (APP_DB_DRIVER,
// default sqlite). An unknown value panics: a typo must fail the run loudly, not
// quietly test the default adapter instead of the one that was asked for.
func ActiveDriver() database.Driver {
	d, err := activeDriver()
	if err != nil {
		panic(fmt.Sprintf("testfx: APP_DB_DRIVER: %v", err))
	}
	return d
}

// MainFor is TestMain for an adapter's own test package: the tests run only when
// the run targets that adapter, so switching APP_DB_DRIVER never leaves a test
// quietly exercising the other database.
//
//	func TestMain(m *testing.M) { testfx.MainFor(m, database.DriverSQLite) }
func MainFor(m *testing.M, d database.Driver) {
	if got := ActiveDriver(); got != d {
		fmt.Printf("skipping %s adapter tests: this run targets APP_DB_DRIVER=%s\n", d, got)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
