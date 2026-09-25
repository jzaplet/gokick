package persistence

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
)

// Postgres is refused loudly until its adapter has repositories — never quietly
// served by another adapter.
func TestOpen_RefusesPostgresUntilItsAdapterIsComplete(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, cleanup, err := Open(&config.Config{DBDriver: database.DriverPostgres}, logger)
	if err == nil || !strings.Contains(err.Error(), `"postgres" is not available yet`) {
		t.Fatalf("Open(postgres): got %v, want the not-available-yet error", err)
	}
	if store != nil || cleanup != nil {
		t.Fatal("a refused driver must yield no store and no cleanup")
	}
}
