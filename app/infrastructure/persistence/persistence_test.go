package persistence

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
)

// A Postgres adapter that cannot be opened fails Open with no store and no
// cleanup — and the error names the variable, never the DSN (it carries a
// password).
func TestOpen_PostgresWithAMalformedDSNFails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const dsn = "postgres://gokick_app:s3cret@[::1"
	store, cleanup, err := Open(
		&config.Config{DBDriver: database.DriverPostgres, DBURL: dsn},
		logger,
	)
	if err == nil || !strings.Contains(err.Error(), "APP_DB_URL") {
		t.Fatalf("Open(postgres): got %v, want an error naming APP_DB_URL", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("the error leaks the DSN: %v", err)
	}
	if store != nil || cleanup != nil {
		t.Fatal("a failed open must yield no store and no cleanup")
	}
}

// An unknown driver is refused, never quietly served by another adapter.
func TestOpen_RefusesAnUnknownDriver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, _, err := Open(&config.Config{DBDriver: database.Driver("oracle")}, logger)
	if err == nil || !strings.Contains(err.Error(), "not built into this binary") {
		t.Fatalf("Open(oracle): got %v, want the not-built error", err)
	}
}
