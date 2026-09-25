package postgres_test

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"gokick/app/infrastructure/postgres"
	"gokick/app/internal/testfx/pgfx"

	"github.com/pressly/goose/v3"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// ownerProvider opens the migration provider over db as the schema owner, and the
// owner connection it runs on.
func ownerProvider(t *testing.T, db *pgfx.DB) (*goose.Provider, *sql.DB) {
	t.Helper()
	owner, err := postgres.OpenOwner(db.OwnerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	p, err := postgres.NewMigrationProvider(owner)
	if err != nil {
		t.Fatal(err)
	}
	return p, owner
}

// assertLatest fails unless db is at the newest embedded migration.
func assertLatest(t *testing.T, db *pgfx.DB) {
	t.Helper()
	p, _ := ownerProvider(t, db)
	current, target, err := p.GetVersions(context.Background())
	if err != nil {
		t.Fatalf("read versions: %v", err)
	}
	if current != target {
		t.Fatalf("version = %d, want the latest %d", current, target)
	}
}

// RunUp migrates an empty database to the latest version and is a no-op the
// second time — Application.Run calls it before every command.
func TestMigrator_RunUpIsIdempotent(t *testing.T) {
	db := pgfx.NewEmpty(t)
	mgr := newManager(t, db.Config())
	m := postgres.NewMigrator(mgr, db.OwnerURL, discardLogger())
	for i := range 2 {
		if err := m.RunUp(); err != nil {
			t.Fatalf("RunUp #%d: %v", i+1, err)
		}
	}
	assertLatest(t, db)
}

// Replicas starting together migrate one at a time (the goose session locker
// takes an advisory lock), so concurrent RunUps all succeed.
func TestMigrator_ConcurrentRunUp(t *testing.T) {
	db := pgfx.NewEmpty(t)
	mgr := newManager(t, db.Config())
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			errs[i] = postgres.NewMigrator(mgr, db.OwnerURL, discardLogger()).RunUp()
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("RunUp #%d: %v", i, err)
		}
	}
	assertLatest(t, db)
}

// Every migration rolls back cleanly and re-applies — the Down sections work.
func TestMigrator_DownToZeroAndUpAgain(t *testing.T) {
	p, owner := ownerProvider(t, pgfx.New(t))
	ctx := context.Background()
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("DownTo(0): %v", err)
	}
	var objects int
	if err := owner.QueryRowContext(ctx, `SELECT count(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname NOT LIKE 'goose_db_version%'`).Scan(&objects); err != nil {
		t.Fatal(err)
	}
	if objects != 0 {
		t.Fatalf("%d relations left in public after rolling everything back", objects)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up after DownTo(0): %v", err)
	}
}

// RunUp is also the startup gate: it refuses a runtime role that would bypass
// the tenant wall, even though the migrations themselves succeed.
func TestMigrator_RunUpRefusesTheOwnerAsTheTenantPlane(t *testing.T) {
	db := pgfx.NewEmpty(t)
	cfg := db.Config()
	cfg.DBURL = db.OwnerURL
	err := postgres.NewMigrator(newManager(t, cfg), db.OwnerURL, discardLogger()).RunUp()
	if err == nil || !strings.Contains(err.Error(), "APP_DB_URL") {
		t.Fatalf("RunUp with the owner as APP_DB_URL: got %v, want a role error", err)
	}
}
