package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/presentation/console"
)

// ---------------------------------------------------------------------------
// Application.Run lifecycle
//
//	overview-09     — Application.Run runs MigrationManager.RunUp() on startup.
//	presentation-02 — migrations are applied BEFORE the subcommand runs, so the
//	                  subcommand already sees the migrated schema.
//
// Both are pinned by one test: Run() is driven against a fresh, *un-migrated*
// temp SQLite DB with a `seed` subcommand whose seeder probes for the `users`
// table. If the probe finds the table, RunUp must have executed before the
// subcommand body ran — which is exactly the ordering application.go promises
// (RunUp -> rootCmd.Execute). Reorder or delete the RunUp call and the probe
// runs against an empty DB and the test fails.
//
// Deliberately does NOT use testfx.New: that fixture migrates the DB itself,
// which would make a "Run migrated it" assertion vacuous. Here the manager is
// built raw and Run() is responsible for the first (and only) RunUp.
// ---------------------------------------------------------------------------

// migrationProbeSeeder stands in for the real seeder. It runs as the body of
// the `seed` subcommand and records whether the `users` table (created by the
// init migration) is present at the moment the subcommand executes.
type migrationProbeSeeder struct {
	db                   *database.SqliteManager
	called               bool
	usersTablePresent    bool
	usersTableQueryError error
}

func (s *migrationProbeSeeder) Seed(ctx context.Context) error {
	s.called = true
	var n int
	// sqlite_master lists every table; the init migration creates `users`.
	s.usersTableQueryError = s.db.DB().GetContext(
		ctx, &n,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='users'",
	)
	s.usersTablePresent = n == 1
	return nil
}

func TestApplicationRun_MigratesBeforeSubcommand(t *testing.T) {
	// No t.Parallel: this test mutates the process-global os.Args (so cobra
	// parses our args instead of the test binary's flags) and RunUp drives
	// goose's process-global SetLogger/SetDialect. Both are racy in parallel.

	dbPath := filepath.Join(t.TempDir(), "app-lifecycle.db")
	manager, err := database.NewSqliteManager(&config.Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	// Sanity: the freshly opened DB must NOT have the schema yet. If it did,
	// the post-Run assertion below would pass even if RunUp never ran, making
	// the test a tautology. Prove the precondition explicitly.
	var before int
	if err := manager.DB().GetContext(
		context.Background(), &before,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='users'",
	); err != nil {
		t.Fatalf("precondition query: %v", err)
	}
	if before != 0 {
		t.Fatalf(
			"precondition: users table already exists before Run (got %d), test would be vacuous",
			before,
		)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	migrations := database.NewMigrationManager(manager, logger)

	probe := &migrationProbeSeeder{db: manager}
	// Only the seed command needs real wiring; the others are nil because
	// .Command() builds the cobra tree without dereferencing their deps
	// (mirrors TestRootCommand_RegistersSubcommands in console).
	rootCmd := console.NewRootCommand(
		console.NewServeCommand(nil, nil, nil),
		console.NewSeedCommand(probe),
		console.NewCreateUserCommand(nil),
		console.NewWorkerCommand(nil),
	)

	application := NewApplication(rootCmd, migrations)

	// RootCommand.Execute -> cobra ExecuteContext, which (lacking SetArgs on the
	// unexported cmd) falls back to os.Args. Point it at the seed subcommand so
	// Run drives RunUp and then executes `seed`.
	origArgs := os.Args
	os.Args = []string{"app", "seed"}
	t.Cleanup(func() { os.Args = origArgs })

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("Application.Run: %v", err)
	}

	// presentation-02: the subcommand actually ran (Run reached Execute after
	// RunUp returned nil).
	if !probe.called {
		t.Fatal("seed subcommand did not run; Run did not reach rootCmd.Execute")
	}
	if probe.usersTableQueryError != nil {
		t.Fatalf("probe query inside subcommand failed: %v", probe.usersTableQueryError)
	}
	// overview-09 + presentation-02: migrations had already created the schema
	// at the moment the subcommand body executed.
	if !probe.usersTablePresent {
		t.Fatal(
			"users table absent when subcommand ran: Run did not apply migrations before the subcommand",
		)
	}
}

// TestApplicationRun_PropagatesMigrationFailure pins the ordering from the other
// side: when RunUp fails, Run must return that error and must NOT proceed to the
// subcommand. A DSN with an invalid journal mode makes NewMigrationManager's
// RunUp target a manager whose pool errors — but more robustly, we force RunUp
// to fail by closing the underlying DB before Run, then assert the subcommand
// never executed. This is the `if err := RunUp(); err != nil { return err }`
// guard at application.go:25-27.
func TestApplicationRun_StopsWhenMigrationFails(t *testing.T) {
	// No t.Parallel — os.Args + goose globals (see test above).

	dbPath := filepath.Join(t.TempDir(), "app-fail.db")
	manager, err := database.NewSqliteManager(&config.Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	migrations := database.NewMigrationManager(manager, logger)

	probe := &migrationProbeSeeder{db: manager}
	rootCmd := console.NewRootCommand(
		console.NewServeCommand(nil, nil, nil),
		console.NewSeedCommand(probe),
		console.NewCreateUserCommand(nil),
		console.NewWorkerCommand(nil),
	)
	application := NewApplication(rootCmd, migrations)

	origArgs := os.Args
	os.Args = []string{"app", "seed"}
	t.Cleanup(func() { os.Args = origArgs })

	// Close the pool so goose's RunUp errors out. Run must surface that error
	// and skip the subcommand.
	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	runErr := application.Run(context.Background())
	if runErr == nil {
		t.Fatal(
			"Run returned nil even though migrations could not run; the RunUp error guard is missing",
		)
	}
	if probe.called {
		t.Fatal(
			"subcommand ran despite a migration failure; Run did not short-circuit on RunUp error",
		)
	}
}
