package app

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"gokick/app/application/bus"
	"gokick/app/presentation/console"
)

// ---------------------------------------------------------------------------
// Application.Run lifecycle
//
//	overview-09     — Application.Run runs Migrator.RunUp() on startup.
//	presentation-02 — migrations are applied BEFORE the subcommand runs, so the
//	                  subcommand already sees the migrated schema.
//
// Both are pinned by one test: Run() is driven with a `seed` subcommand whose
// seeder and the migrator write into one shared call log. If the log reads
// [migrate seed], RunUp executed before the subcommand body ran — which is
// exactly the ordering application.go promises (RunUp -> rootCmd.Execute).
// Reorder or delete the RunUp call and the log comes out wrong.
//
// The migrator is a stand-in on purpose: what is under test is Run's ordering,
// not the migration set (every testfx fixture applies that on the active
// adapter), and a stand-in keeps this test independent of any database.
// ---------------------------------------------------------------------------

// callLog records the order in which the migrator and the subcommand ran.
type callLog []string

// recordingMigrator stands in for the adapter's migrator: it logs the call and
// returns err.
type recordingMigrator struct {
	log *callLog
	err error
}

func (m recordingMigrator) RunUp() error {
	*m.log = append(*m.log, "migrate")
	return m.err
}

// probeSeeder stands in for the real seeder and runs as the body of the `seed`
// subcommand.
type probeSeeder struct{ log *callLog }

func (s probeSeeder) Seed(context.Context) error {
	*s.log = append(*s.log, "seed")
	return nil
}

// newSeedApplication builds an Application whose only live subcommand is `seed`
// and points os.Args at it. The other commands are nil-wired: .Command() builds
// the cobra tree without dereferencing their deps (mirrors
// TestRootCommand_RegistersSubcommands in console).
func newSeedApplication(t *testing.T, log *callLog, migrateErr error) *Application {
	t.Helper()
	rootCmd := console.NewRootCommand(
		console.NewServeCommand(nil, nil, nil),
		// seed dispatches through the SystemCommandBus; a bare one runs the probe.
		console.NewSeedCommand(probeSeeder{log: log}, bus.NewSystemCommandBus()),
		console.NewCreateUserCommand(nil, nil, nil, nil, nil),
		console.NewCreateSuperAdminCommand(nil, nil),
		console.NewCreateTenantCommand(nil, nil),
		console.NewWorkerCommand(nil),
	)

	// RootCommand.Execute -> cobra ExecuteContext, which (lacking SetArgs on the
	// unexported cmd) falls back to os.Args. Point it at the seed subcommand so
	// Run drives RunUp and then executes `seed`.
	origArgs := os.Args
	os.Args = []string{"app", "seed"}
	t.Cleanup(func() { os.Args = origArgs })

	return NewApplication(rootCmd, recordingMigrator{log: log, err: migrateErr})
}

func TestApplicationRun_MigratesBeforeSubcommand(t *testing.T) {
	// No t.Parallel: this test mutates the process-global os.Args (so cobra
	// parses our args instead of the test binary's flags), which is racy in
	// parallel.
	var log callLog
	application := newSeedApplication(t, &log, nil)

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("Application.Run: %v", err)
	}

	// overview-09 + presentation-02: migrations ran, then the subcommand ran.
	if want := (callLog{"migrate", "seed"}); !slices.Equal(log, want) {
		t.Fatalf("call order: got %v want %v — Run must migrate before the subcommand", log, want)
	}
}

// TestApplicationRun_StopsWhenMigrationFails pins the ordering from the other
// side: when RunUp fails, Run must return that error and must NOT proceed to the
// subcommand. This is the `if err := RunUp(); err != nil { return err }` guard in
// application.go.
func TestApplicationRun_StopsWhenMigrationFails(t *testing.T) {
	// No t.Parallel — os.Args (see test above).
	migrateErr := errors.New("migrations failed")
	var log callLog
	application := newSeedApplication(t, &log, migrateErr)

	runErr := application.Run(context.Background())
	if !errors.Is(runErr, migrateErr) {
		t.Fatalf(
			"Run returned %v, want the migration error; the RunUp error guard is missing",
			runErr,
		)
	}
	if slices.Contains(log, "seed") {
		t.Fatal(
			"subcommand ran despite a migration failure; Run did not short-circuit on RunUp error",
		)
	}
}
