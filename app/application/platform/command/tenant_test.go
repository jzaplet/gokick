package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/internal/testfx"
)

// The rule the whole tenant-delete feature stands on: a tenant that still owns
// users survives. The UI disables the button, but the button is a hint over a
// count that was stale the moment it rendered — this is the gate that actually
// holds.
func TestDeleteTenant_RefusesTenantWithUsers(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_busy.db"))

	busy := fx.SeedTenant(t, "Beta")
	fx.SeedUserInTenant(t, "bob", "user", busy.ID)

	h := NewDeleteTenantHandler(fx.PlatformTenants)
	err := h.Handle(ctx, DeleteTenantCommand{ID: busy.ID})
	if err == nil {
		t.Fatal("deleting a tenant that still has users must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError (400), got %T: %v", err, err)
	}

	// The refusal must be a refusal, not a message over a completed delete.
	got, err := fx.Tenants.FindByID(ctx, busy.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the tenant must still exist after a refused delete")
	}
}

// A tenant owns more than its users. runs.tenant_id carries NO foreign key, so
// nothing under the gate would refuse a widowed run: the tenant would go, the run
// would survive pointing at nothing, and the worker would later restore a dead
// tenant id into the handler ctx — where scoped reads match zero rows WITHOUT
// erroring, so a resumed run can "succeed" against an empty world.
//
// The users-first rule is what makes this reachable: a tenant with users is
// refused, so emptying it is the required first step, and that is exactly the step
// that strands its runs.
func TestDeleteTenant_RefusesTenantWithUnfinishedRuns(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_runs.db"))

	busy := fx.SeedTenant(t, "Exporting")
	fx.SeedRunInTenant(t, "e2e:noop", busy.ID)

	h := NewDeleteTenantHandler(fx.PlatformTenants)
	err := h.Handle(ctx, DeleteTenantCommand{ID: busy.ID})
	if err == nil {
		t.Fatal("a tenant that still has an unfinished run must not be deletable")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError (400), got %T: %v", err, err)
	}

	got, err := fx.Tenants.FindByID(ctx, busy.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the tenant must survive so its run keeps a live tenant to point at")
	}
}

// The flip side, and the reason the gate tests NON-TERMINAL runs rather than any
// run at all: a finished run is history. It is never claimed again, so it cannot
// resume under a dead tenant and must not pin the tenant forever.
func TestDeleteTenant_TerminalRunsDoNotPinTheTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_runs_done.db"))

	done := fx.SeedTenant(t, "Finished")
	r := fx.SeedRunInTenant(t, "e2e:noop", done.ID)
	fx.MarkRunCompleted(t, r.ID)

	h := NewDeleteTenantHandler(fx.PlatformTenants)
	if err := h.Handle(ctx, DeleteTenantCommand{ID: done.ID}); err != nil {
		t.Fatalf("a tenant whose only runs are finished must be deletable: %v", err)
	}

	got, err := fx.Tenants.FindByID(ctx, done.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Fatal("the tenant must be gone")
	}
}

func TestDeleteTenant_DeletesEmptyTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_empty.db"))

	empty := fx.SeedTenant(t, "Ghost")

	h := NewDeleteTenantHandler(fx.PlatformTenants)
	if err := h.Handle(ctx, DeleteTenantCommand{ID: empty.ID}); err != nil {
		t.Fatalf("an empty tenant must be deletable: %v", err)
	}

	got, err := fx.Tenants.FindByID(ctx, empty.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Fatal("the tenant must be gone")
	}
}

// The default tenant is refused by IDENTITY, not merely because it usually holds
// a superadmin. Single-tenant mode puts every user in it and runs.tenant_id
// DEFAULTs to its id, so deleting it would strand rows — the guard must hold even
// on the empty tenant this test constructs, where the user-count rule would not.
//
// It asserts the MESSAGE, not just survival, and that is the whole point. There are
// now two floors under the default tenant — this handler's identity check and the
// repository's `id != ?` — so "it survived" is proved by the repo alone and would
// stay true with this guard deleted. What the guard uniquely owns is the honest
// answer: without it the delete falls through to the generic branch and tells an
// operator to remove users from a tenant that has none. Survival is the repo's
// test (see TestDeleteIfEmptyAcrossTenants_RefusesTheDefaultTenantInSQL); the
// reason is this one's.
func TestDeleteTenant_RefusesDefaultTenantEvenWhenEmpty(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_default.db"))

	// No users, no runs: the default tenant owns nothing here, so the emptiness
	// rule would happily let it go — only identity refuses it.
	h := NewDeleteTenantHandler(fx.PlatformTenants)
	err := h.Handle(ctx, DeleteTenantCommand{ID: shared.DefaultTenantID})
	if err == nil {
		t.Fatal("the default tenant must never be deletable")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Key != msgkey.TenantDefaultUndeletable {
		t.Fatalf("the refusal must name the real reason (identity), got %q", ve.Key)
	}

	got, err := fx.Tenants.FindByID(ctx, shared.DefaultTenantID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the default tenant must survive")
	}
}

// An unknown tenant is reported FIELDLESSLY on purpose. The id is a path param and
// the grid has no field to route an `id` key to, so Responder would send it to a
// key nothing on the screen reads and the operator would see only a generic
// "Failed to delete the tenant." — the message would exist and never be shown.
// Fieldless goes to `general`, which the toast reads.
func TestDeleteTenant_UnknownTenantIsNotFound(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_404.db"))

	h := NewDeleteTenantHandler(fx.PlatformTenants)
	err := h.Handle(ctx, DeleteTenantCommand{ID: "01920000-0000-7000-8000-000000000000"})
	if err == nil {
		t.Fatal("an unknown tenant must be reported, not silently succeed")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "" {
		t.Fatalf("the refusal must be fieldless so it reaches the toast, got field %q", ve.Field)
	}
}

// The floor under the handler's identity check: the repository refuses the default
// tenant in the statement itself, the way its bulk sibling already does. The test
// calls the repository DIRECTLY and deliberately — through the handler it would
// pass with this floor deleted, because the handler refuses DefaultTenantID before
// the repository is ever reached. That is the point: a second caller that skips the
// handler (a cleanup job, a CLI delete-tenant) inherits the emptiness rule for
// free, and this is what stops it inheriting a hole with it.
//
// It lives beside the handler tests rather than in sqlite/tenant/ because testfx
// imports that package — an internal test there would be an import cycle.
func TestDeleteIfEmptyAcrossTenants_RefusesTheDefaultTenantInSQL(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_repo_default.db"))

	// No users seeded, so the emptiness condition would happily let this through:
	// the identity floor is the only thing that can refuse it here.
	deleted, err := fx.PlatformTenants.DeleteIfEmptyAcrossTenants(ctx, shared.DefaultTenantID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted {
		t.Fatal("the repository must never report the default tenant as deleted")
	}

	got, err := fx.Tenants.FindByID(ctx, shared.DefaultTenantID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the default tenant must survive a direct repository delete")
	}
}

// Bulk is partial by design: the empty tenants in the selection go, the ones that
// still have users stay, and `affected` counts only what actually happened. A
// selection mixing both must not be all-or-nothing in either direction.
func TestBulkDeleteTenants_DeletesOnlyTheEmptyOnes(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_mixed.db"))

	empty1 := fx.SeedTenant(t, "Ghost One")
	empty2 := fx.SeedTenant(t, "Ghost Two")
	busy := fx.SeedTenant(t, "Busy")
	fx.SeedUserInTenant(t, "bob", "user", busy.ID)

	h := NewBulkDeleteTenantsHandler(fx.PlatformTenants)
	affected, err := h.Handle(ctx, BulkDeleteTenantsCommand{
		IDs: []string{empty1.ID, empty2.ID, busy.ID},
	})
	if err != nil {
		t.Fatalf("a mixed selection must not error: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected must count only the deleted tenants, got %d want 2", affected)
	}

	for _, id := range []string{empty1.ID, empty2.ID} {
		got, _ := fx.Tenants.FindByID(ctx, id)
		if got != nil {
			t.Fatalf("empty tenant %q must be gone", id)
		}
	}
	if got, _ := fx.Tenants.FindByID(ctx, busy.ID); got == nil {
		t.Fatal("the tenant with users must survive a bulk delete")
	}
}

// The default tenant is spared in bulk too — including all-filtered mode, where
// nobody enumerated an id and the statement is the only thing standing between a
// broad selection and the tenant the whole single-tenant mode rests on.
func TestBulkDeleteTenants_SparesDefaultTenantWhenAllFiltered(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_default.db"))

	victim := fx.SeedTenant(t, "Ghost")

	h := NewBulkDeleteTenantsHandler(fx.PlatformTenants)
	affected, err := h.Handle(ctx, BulkDeleteTenantsCommand{AllFiltered: true})
	if err != nil {
		t.Fatalf("all-filtered bulk delete: %v", err)
	}

	if got, _ := fx.Tenants.FindByID(ctx, shared.DefaultTenantID); got == nil {
		t.Fatal("the default tenant must survive an unfiltered bulk delete")
	}
	if got, _ := fx.Tenants.FindByID(ctx, victim.ID); got != nil {
		t.Fatal("a selected empty tenant should still have been deleted")
	}
	if affected != 1 {
		t.Fatalf("only the one empty non-default tenant should go, got affected=%d", affected)
	}
}

// All-filtered mode must delete exactly the set the grid showed — the filters are
// what the superadmin actually saw, so a filter that fails to reach the statement
// silently widens the blast radius.
func TestBulkDeleteTenants_AllFilteredHonoursTheNameFilter(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_filter.db"))

	matching := fx.SeedTenant(t, "Ghost One")
	other := fx.SeedTenant(t, "Keep Me")

	h := NewBulkDeleteTenantsHandler(fx.PlatformTenants)
	affected, err := h.Handle(ctx, BulkDeleteTenantsCommand{
		AllFiltered: true,
		Name:        "Ghost",
	})
	if err != nil {
		t.Fatalf("filtered bulk delete: %v", err)
	}
	if affected != 1 {
		t.Fatalf("only the filtered tenant should go, got affected=%d", affected)
	}

	if got, _ := fx.Tenants.FindByID(ctx, matching.ID); got != nil {
		t.Fatal("the tenant matching the filter must be deleted")
	}
	if got, _ := fx.Tenants.FindByID(ctx, other.ID); got == nil {
		t.Fatal("a tenant OUTSIDE the filter must never be touched")
	}
}

// The Plan filter has to reach the statement for the same reason Name does, and it
// gets its own test because it is the half a Name-only test cannot speak for: a
// selection built from `ListFilters{Name: cmd.Name}` — one plausible slip — passes
// every other test in this file while turning "delete the free-plan tenants" into
// "delete every empty tenant in the install". The grid offers the plan filter, so
// this is a path a superadmin can actually take.
func TestBulkDeleteTenants_AllFilteredHonoursThePlanFilter(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_plan.db"))

	free := fx.SeedTenant(t, "Free One")
	paid := fx.SeedTenantWithPlan(t, "Paid One", "pro")

	h := NewBulkDeleteTenantsHandler(fx.PlatformTenants)
	affected, err := h.Handle(ctx, BulkDeleteTenantsCommand{
		AllFiltered: true,
		Plan:        "free",
	})
	if err != nil {
		t.Fatalf("plan-filtered bulk delete: %v", err)
	}
	if affected != 1 {
		t.Fatalf("only the free-plan tenant should go, got affected=%d", affected)
	}

	if got, _ := fx.Tenants.FindByID(ctx, free.ID); got != nil {
		t.Fatal("the tenant matching the plan filter must be deleted")
	}
	if got, _ := fx.Tenants.FindByID(ctx, paid.ID); got == nil {
		t.Fatal("a tenant OUTSIDE the plan filter must never be touched")
	}
}

func TestBulkDeleteTenants_EmptySelectionIsRefused(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_none.db"))

	h := NewBulkDeleteTenantsHandler(fx.PlatformTenants)
	_, err := h.Handle(ctx, BulkDeleteTenantsCommand{})
	if err == nil {
		t.Fatal("an empty selection must be refused, not treated as all-filtered")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
}
