package console

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	jobapp "gokick/app/application/job"
	usercmd "gokick/app/application/user/command"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/worker"
	"gokick/app/internal/testfx"

	"github.com/spf13/cobra"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestWorker builds a real Worker the cheap way the worker package's own
// tests do — an empty handler registry plus the throwaway dispatcher returned
// for a bare context. Its Run drains promptly once ctx is cancelled.
func newTestWorker(t *testing.T, fx *testfx.Fixture) *worker.Worker {
	t.Helper()
	registry, err := jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	dispatcher := shared.JobDispatcherFromContext(context.Background())
	return worker.NewWorker(
		silentLogger(),
		shared.NopReporter{},
		fx.Jobs,
		registry,
		fx.DB,
		dispatcher,
		1,
	)
}

// ---------------------------------------------------------------------------
// create-user command wiring
//   overview-107  — flags -n/-p, optional -e/-r, default role "admin"
//   presentation-07 — shorthands, role default "admin", required nickname+password
//   infra-db-security-19 — arbitrary role created via create-user (-r user)
// ---------------------------------------------------------------------------

func TestCreateUserCommand_FlagSpec(t *testing.T) {
	t.Parallel()

	cmd := NewCreateUserCommand(nil).Command()

	if got := cmd.Use; got != "create-user" {
		t.Fatalf("Use: got %q want %q", got, "create-user")
	}

	cases := []struct {
		name      string
		shorthand string
	}{
		{"nickname", "n"},
		{"password", "p"},
		{"email", "e"},
		{"role", "r"},
	}
	for _, c := range cases {
		f := cmd.Flags().Lookup(c.name)
		if f == nil {
			t.Fatalf("flag %q not registered", c.name)
		}
		if f.Shorthand != c.shorthand {
			t.Fatalf("flag %q shorthand: got %q want %q", c.name, f.Shorthand, c.shorthand)
		}
	}

	// role defaults to "admin".
	if def := cmd.Flags().Lookup("role").DefValue; def != "admin" {
		t.Fatalf("role default: got %q want %q", def, "admin")
	}

	// nickname and password are required; email and role are not.
	assertRequired(t, cmd, "nickname", true)
	assertRequired(t, cmd, "password", true)
	assertRequired(t, cmd, "email", false)
	assertRequired(t, cmd, "role", false)
}

func TestCreateUserCommand_MissingRequiredFlagErrors(t *testing.T) {
	// No t.Parallel: testfx.New runs goose migrations, which set process-global
	// state (SetLogger/SetDialect) — concurrent New() calls race under -race.
	// The rest of the codebase keeps testfx-backed tests serial for this reason.
	fx := testfx.New(t, filepath.Join(t.TempDir(), "console.db"))
	handler := usercmd.NewCreateUserHandler(fx.Users, fx.Hasher)
	cmd := NewCreateUserCommand(handler).Command()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Omit -p (password). cobra must reject before RunE runs.
	cmd.SetArgs([]string{"-n", "alice"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected error when required --password flag is missing, got nil")
	}

	// Nothing should have been persisted.
	got, err := fx.Users.FindByNickname(context.Background(), "alice")
	if err != nil {
		t.Fatalf("FindByNickname: %v", err)
	}
	if got != nil {
		t.Fatalf("no user must be created when a required flag is missing, got %+v", got)
	}
}

func TestCreateUserCommand_DefaultsRoleToAdmin(t *testing.T) {
	// No t.Parallel — see TestCreateUserCommand_MissingRequiredFlagErrors (goose globals).
	fx := testfx.New(t, filepath.Join(t.TempDir(), "console.db"))
	handler := usercmd.NewCreateUserHandler(fx.Users, fx.Hasher)
	cmd := NewCreateUserCommand(handler).Command()
	cmd.SilenceUsage = true

	// No -r flag -> role must default to "admin".
	cmd.SetArgs([]string{"-n", "alice", "-p", "secret12"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute create-user: %v", err)
	}

	u, err := fx.Users.FindByNickname(context.Background(), "alice")
	if err != nil {
		t.Fatalf("FindByNickname: %v", err)
	}
	if u == nil {
		t.Fatal("user was not created")
	}
	if u.Role != "admin" {
		t.Fatalf("default role: got %q want %q", u.Role, "admin")
	}
}

func TestCreateUserCommand_RoleFlagCreatesUserRole(t *testing.T) {
	// No t.Parallel — see TestCreateUserCommand_MissingRequiredFlagErrors (goose globals).
	fx := testfx.New(t, filepath.Join(t.TempDir(), "console.db"))
	handler := usercmd.NewCreateUserHandler(fx.Users, fx.Hasher)
	cmd := NewCreateUserCommand(handler).Command()
	cmd.SilenceUsage = true

	// Explicit -r user -> an arbitrary (non-admin) role is created.
	cmd.SetArgs([]string{"-n", "bob", "-p", "secret12", "-e", "bob@example.com", "-r", "user"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute create-user: %v", err)
	}

	u, err := fx.Users.FindByNickname(context.Background(), "bob")
	if err != nil {
		t.Fatalf("FindByNickname: %v", err)
	}
	if u == nil {
		t.Fatal("user was not created")
	}
	if u.Role != "user" {
		t.Fatalf("role: got %q want %q", u.Role, "user")
	}
	if u.Email != "bob@example.com" {
		t.Fatalf("email: got %q want %q", u.Email, "bob@example.com")
	}
}

// ---------------------------------------------------------------------------
// root command wiring
//   presentation-01 — root exposes serve, seed, create-user, worker subcommands
// ---------------------------------------------------------------------------

func TestRootCommand_RegistersSubcommands(t *testing.T) {
	t.Parallel()

	// .Command() builds the cobra tree without dereferencing the injected
	// dependencies, so nil deps are safe for inspecting the command set.
	root := NewRootCommand(
		NewServeCommand(nil, nil, nil),
		NewSeedCommand(nil),
		NewCreateUserCommand(nil),
		NewWorkerCommand(nil),
	)

	names := map[string]bool{}
	for _, sub := range root.cmd.Commands() {
		names[sub.Name()] = true
	}

	for _, want := range []string{"serve", "seed", "create-user", "worker"} {
		if !names[want] {
			t.Fatalf("root command missing subcommand %q; registered: %v", want, names)
		}
	}
}

// ---------------------------------------------------------------------------
// seed command delegation
//   presentation-04 (partial) — RunE calls seeder.Seed with cmd's context
// ---------------------------------------------------------------------------

type ctxKey string

type recordingSeeder struct {
	called bool
	saw42  bool
}

func (s *recordingSeeder) Seed(ctx context.Context) error {
	s.called = true
	if v, ok := ctx.Value(ctxKey("k")).(int); ok && v == 42 {
		s.saw42 = true
	}
	return nil
}

func TestSeedCommand_DelegatesToSeederWithContext(t *testing.T) {
	t.Parallel()

	seeder := &recordingSeeder{}
	cmd := NewSeedCommand(seeder).Command()
	cmd.SilenceUsage = true

	// Tag the context so we can prove RunE forwarded *this* context to Seed.
	ctx := context.WithValue(context.Background(), ctxKey("k"), 42)
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute seed: %v", err)
	}

	if !seeder.called {
		t.Fatal("seed command did not invoke seeder.Seed")
	}
	if !seeder.saw42 {
		t.Fatal("seed command did not forward cmd.Context() to seeder.Seed")
	}
}

// ---------------------------------------------------------------------------
// worker command lifecycle
//   roadmap-41 (worker half) — standalone worker command's RunE forwards
//   cmd.Context() to worker.Run, so cancelling the context drains it.
// ---------------------------------------------------------------------------

func TestWorkerCommand_RunDrainsOnContextCancel(t *testing.T) {
	// No t.Parallel — see TestCreateUserCommand_MissingRequiredFlagErrors (goose globals).
	fx := testfx.New(t, filepath.Join(t.TempDir(), "console.db"))
	w := newTestWorker(t, fx)

	cmd := NewWorkerCommand(w).Command()
	cmd.SilenceUsage = true

	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)

	done := make(chan error, 1)
	go func() { done <- cmd.RunE(cmd, nil) }()

	// If RunE wired cmd.Context() into worker.Run, cancelling unblocks it.
	// (If it passed a fresh/background context instead, this would hang and
	// the test would fail on timeout — that's the assertion.)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker RunE returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal(
			"worker RunE did not return after context cancel — cmd.Context() not forwarded to worker.Run",
		)
	}
}

// --- assertion / construction helpers --------------------------------------

func assertRequired(t *testing.T, cmd *cobra.Command, flag string, want bool) {
	t.Helper()
	f := cmd.Flags().Lookup(flag)
	if f == nil {
		t.Fatalf("flag %q not registered", flag)
	}
	// cobra records required flags via the BashCompOneRequiredFlag annotation.
	_, required := f.Annotations[cobra.BashCompOneRequiredFlag]
	if required != want {
		t.Fatalf("flag %q required: got %v want %v", flag, required, want)
	}
}
