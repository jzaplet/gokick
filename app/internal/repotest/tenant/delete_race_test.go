package tenant_test

import (
	"context"
	"testing"
	"time"

	"gokick/app/domain/run"
	"gokick/app/domain/tenant"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// A tenant delete races an insert into that tenant: a user or a pending run whose
// transaction has not committed yet when the delete starts, so the delete's
// snapshot cannot see it. Once the insert commits, the delete must refuse the
// tenant — not fail on the users foreign key (a 500), and not delete the tenant
// and take the new run with it (runs.tenant_id cascades on Postgres). SQLite
// makes the delete wait for the writer and then see the row; Postgres needs the
// delete to lock the tenant first and decide in a statement of its own.
func TestTenantDelete_RacingInsertKeepsTheTenant(t *testing.T) {
	deletes := map[string]func(ctx context.Context, fx *testfx.Fixture, id string) (bool, error){
		"single": func(ctx context.Context, fx *testfx.Fixture, id string) (bool, error) {
			return fx.PlatformTenants.DeleteIfEmptyAcrossTenants(ctx, id)
		},
		"bulk": func(ctx context.Context, fx *testfx.Fixture, id string) (bool, error) {
			ids, err := fx.PlatformTenants.BulkDeleteEmptyAcrossTenants(ctx,
				tenant.BulkSelection{IDs: []string{id}})
			return len(ids) > 0, err
		},
	}
	inserts := map[string]struct {
		table  string
		insert func(ctx context.Context, fx *testfx.Fixture, tenantID string) error
	}{
		"user": {"users", func(ctx context.Context, fx *testfx.Fixture, tenantID string) error {
			u := user.NewUser("racer", "hash", "", user.RoleUser, tenantID)
			return fx.Users.Save(ctx, u)
		}},
		"run": {"runs", func(ctx context.Context, fx *testfx.Fixture, tenantID string) error {
			r, err := run.NewRun("race", []byte(`{}`), 0)
			if err != nil {
				return err
			}
			r.TenantID = tenantID
			return fx.Runs.Enqueue(ctx, r)
		}},
	}

	for delName, del := range deletes {
		for insName, ins := range inserts {
			t.Run(delName+" delete vs "+insName+" insert", func(t *testing.T) {
				fx := testfx.New(t)
				tn := fx.SeedTenant(t, "Racing")

				deleted, err := deleteWhileInserting(t, fx,
					func(ctx context.Context) error { return ins.insert(ctx, fx, tn.ID) },
					func(ctx context.Context) (bool, error) { return del(ctx, fx, tn.ID) })
				if err != nil {
					t.Fatalf("the delete failed instead of refusing the tenant: %v", err)
				}
				if deleted {
					t.Fatal("the delete removed a tenant a concurrent insert had just filled")
				}
				if n := fx.Count(t, "tenants", "id = ?", tn.ID); n != 1 {
					t.Fatalf("tenant rows = %d, want the tenant kept", n)
				}
				if n := fx.Count(t, ins.table, "tenant_id = ?", tn.ID); n != 1 {
					t.Fatalf("%s rows in the tenant = %d, want the inserted one kept", ins.table, n)
				}
			})
		}
	}
}

// deleteWhileInserting runs insert in a transaction it leaves open, starts del
// alongside it, commits the insert once del has had time to reach the tenant and
// wait on the insert, and returns del's outcome. A slow start of del only weakens
// the race (del then sees the committed row), it never fails the test falsely.
func deleteWhileInserting(
	t *testing.T,
	fx *testfx.Fixture,
	insert func(ctx context.Context) error,
	del func(ctx context.Context) (bool, error),
) (bool, error) {
	t.Helper()
	txCtx, err := fx.Tx.BeginTx(testfx.SystemCtx())
	if err != nil {
		t.Fatalf("begin the insert: %v", err)
	}
	if err := insert(txCtx); err != nil {
		_ = fx.Tx.Rollback(txCtx)
		t.Fatalf("insert: %v", err)
	}

	type outcome struct {
		deleted bool
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		deleted, err := del(testfx.PlatformCtx())
		done <- outcome{deleted, err}
	}()

	select {
	case o := <-done:
		_ = fx.Tx.Rollback(txCtx)
		t.Fatalf("the delete (%v, %v) did not wait for an insert into its tenant", o.deleted, o.err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := fx.Tx.Commit(txCtx); err != nil {
		t.Fatalf("commit the insert: %v", err)
	}
	o := <-done
	return o.deleted, o.err
}
