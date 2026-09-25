package di

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/database"
	"gokick/app/internal/testfx"
)

// Two commands that lock the same two rows in opposite order deadlock. Postgres
// breaks the cycle by aborting one of them (40P01), and the CommandBus runs that
// one again in a fresh transaction: both commands succeed, and each dispatches
// its event and writes its audit row exactly once — the aborted attempt leaves
// nothing behind. Postgres only: SQLite serializes write transactions at BEGIN,
// so the two commands never interleave and the barrier below would wait forever.
func TestCommandBus_RetriesADeadlockedCommand(t *testing.T) {
	if testfx.ActiveDriver() != database.DriverPostgres {
		t.Skip("SQLite serializes write transactions at BEGIN: two commands cannot deadlock")
	}
	fx := testfx.New(t)
	alpha := fx.SeedUser(t, "alpha", "password123", "user")
	bravo := fx.SeedUser(t, "bravo", "password123", "user")

	var fired atomic.Int32
	cmdBus := newProductionCommandBus(t, fx, noopDispatcher{}, countTestEvents(&fired))

	// Each command's first attempt locks its first row, then waits until the
	// other holds its own first row too — so the second locks close the cycle.
	var firstLocks sync.WaitGroup
	firstLocks.Add(2)
	var attempts atomic.Int32
	updateBoth := func(action string, first, second user.User) func(context.Context) error {
		tries := 0
		return func(ctx context.Context) error {
			tries++
			attempts.Add(1)
			shared.EventCollectorFromContext(ctx).Collect(testEvent{})
			shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: action})
			if err := fx.Users.Update(ctx, &first); err != nil {
				return err
			}
			if tries == 1 {
				firstLocks.Done()
				firstLocks.Wait()
			}
			return fx.Users.Update(ctx, &second)
		}
	}

	errs := make(chan error, 2)
	for _, c := range []struct {
		action        string
		first, second *user.User
	}{
		{"test.alpha_then_bravo", alpha, bravo},
		{"test.bravo_then_alpha", bravo, alpha},
	} {
		go func() {
			errs <- bus.DispatchVoid(context.Background(), cmdBus, c.action, skipPermCmd{},
				updateBoth(c.action, *c.first, *c.second))
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("a command failed instead of retrying its deadlock: %v", err)
		}
	}

	if got := attempts.Load(); got != 3 {
		t.Fatalf("handler attempts = %d, want 3 (the deadlock victim runs twice)", got)
	}
	if got := fired.Load(); got != 2 {
		t.Fatalf("events dispatched = %d, want one per command", got)
	}
	for _, action := range []string{"test.alpha_then_bravo", "test.bravo_then_alpha"} {
		if n := fx.Count(t, "audit_log", "action = ?", action); n != 1 {
			t.Fatalf("audit rows for %s = %d, want exactly one", action, n)
		}
	}
}
