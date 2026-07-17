package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// A superadmin edits a user in ANOTHER tenant: the cross-tenant write lands, the
// tenant is NOT changed, and `active` is PRESERVED (the form carries no active
// flag, so the command must not deactivate the user — the advisor's regression).
func TestUpdatePlatformUser_CrossTenant_PreservesActiveAndTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_update.db"))

	tenantB := fx.SeedTenant(t, "Beta")
	victim := fx.SeedUserInTenant(t, "bob", "user", tenantB.ID) // active=true

	h := NewUpdatePlatformUserHandler(fx.PlatformUsers, fx.Hasher)
	err := h.Handle(ctx, UpdatePlatformUserCommand{
		ID:       victim.ID,
		Nickname: "bobby",
		Email:    "bobby@example.com",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("cross-tenant edit must land: %v", err)
	}

	got, err := fx.Users.FindByID(ctx, victim.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Nickname != "bobby" || got.Role != "admin" {
		t.Fatalf("edit must persist, got nick=%q role=%q", got.Nickname, got.Role)
	}
	if got.Active != true {
		t.Fatal("active must be preserved across a platform edit (form sends no active flag)")
	}
	if got.TenantID != tenantB.ID {
		t.Fatalf("edit must NOT move the user between tenants, got %q", got.TenantID)
	}
}

// Editing a SUPERADMIN target is rejected (it is managed out-of-band only).
func TestUpdatePlatformUser_RejectsSuperadminTarget(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_update_super.db"))

	super := fx.SeedUserInTenant(t, "root", "superadmin", shared.DefaultTenantID)

	h := NewUpdatePlatformUserHandler(fx.PlatformUsers, fx.Hasher)
	err := h.Handle(ctx, UpdatePlatformUserCommand{
		ID:       super.ID,
		Nickname: "hax",
		Email:    "h@x.com",
		Role:     "admin",
	})
	if err == nil {
		t.Fatal("editing a superadmin must be rejected")
	}
	var pe *shared.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *shared.PermissionError, got %T: %v", err, err)
	}

	got, _ := fx.Users.FindByID(ctx, super.ID)
	if got.Nickname != "root" || got.Role != "superadmin" {
		t.Fatal("superadmin must be untouched after a rejected edit")
	}
}

// Promoting anyone to superadmin via the platform edit is rejected.
func TestUpdatePlatformUser_RejectsPromotionToSuperadmin(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_update_promote.db"))

	victim := fx.SeedUserInTenant(t, "bob", "user", shared.DefaultTenantID)

	h := NewUpdatePlatformUserHandler(fx.PlatformUsers, fx.Hasher)
	err := h.Handle(ctx, UpdatePlatformUserCommand{
		ID:       victim.ID,
		Nickname: "bob",
		Email:    "bob@example.com",
		Role:     "superadmin",
	})
	if err == nil {
		t.Fatal("promotion to superadmin via the platform edit must be rejected")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected role ValidationError, got %T: %v", err, err)
	}
}

// Cross-tenant delete lands for a regular user; a superadmin target is rejected.
func TestDeletePlatformUser_CrossTenantAndSuperadminGuard(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_delete.db"))

	tenantB := fx.SeedTenant(t, "Beta")
	victim := fx.SeedUserInTenant(t, "bob", "user", tenantB.ID)
	super := fx.SeedUserInTenant(t, "root", "superadmin", shared.DefaultTenantID)

	// Caller is a (distinct) superadmin identity.
	callerCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: "caller-super", Role: "superadmin",
	})

	h := NewDeletePlatformUserHandler(fx.PlatformUsers)

	// A superadmin target is rejected (managed out-of-band).
	if err := h.Handle(callerCtx, DeletePlatformUserCommand{ID: super.ID}); err == nil {
		t.Fatal("deleting a superadmin must be rejected")
	}
	if got, _ := fx.Users.FindByID(ctx, super.ID); got == nil {
		t.Fatal("superadmin must survive a rejected delete")
	}

	// A regular user in another tenant is deleted.
	if err := h.Handle(callerCtx, DeletePlatformUserCommand{ID: victim.ID}); err != nil {
		t.Fatalf("cross-tenant delete must land: %v", err)
	}
	if got, _ := fx.Users.FindByID(ctx, victim.ID); got != nil {
		t.Fatal("victim must be deleted cross-tenant")
	}
}

// The platform write commands must be DENIED at the BUS for an admin identity —
// proving they actually demand a platform:* permission, not just that the
// handler guards work. Without this, fat-fingering RequiredPermission() to an
// admin:* string would hand an admin cross-tenant write with every other test
// still green. (The inverse-direction guard for the write path.)
func TestPlatformWriteCommands_AdminDeniedAtBus(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_write_authz.db"))
	victim := fx.SeedUserInTenant(t, "bob", "user", shared.DefaultTenantID)
	cmdBus, _, _ := fx.NewBuses()

	adminCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{UserID: "a1", Role: "admin"})

	uh := NewUpdatePlatformUserHandler(fx.PlatformUsers, fx.Hasher)
	updateCmd := UpdatePlatformUserCommand{
		ID: victim.ID, Nickname: "hax", Email: "h@x.com", Role: "user",
	}
	_, updErr := testfx.ExecCommand(adminCtx, cmdBus, "PlatformUpdateUser", updateCmd,
		func(ctx context.Context) (any, error) { return nil, uh.Handle(ctx, updateCmd) })
	assertPermissionDenied(t, "update", updErr)

	dh := NewDeletePlatformUserHandler(fx.PlatformUsers)
	deleteCmd := DeletePlatformUserCommand{ID: victim.ID}
	_, delErr := testfx.ExecCommand(adminCtx, cmdBus, "PlatformDeleteUser", deleteCmd,
		func(ctx context.Context) (any, error) { return nil, dh.Handle(ctx, deleteCmd) })
	assertPermissionDenied(t, "delete", delErr)

	// Create is the one that matters most: it writes through SaveAcrossTenants,
	// which is deliberately exempt from the scope guard. The permission string is
	// the ONLY thing standing between an admin and a row planted in any tenant.
	ch := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
	createCmd := CreatePlatformUserCommand{
		Nickname: "mallory",
		Password: "correct horse battery staple",
		Email:    "m@x.com",
		Role:     "admin",
		TenantID: shared.DefaultTenantID,
	}
	_, createErr := testfx.ExecCommand(adminCtx, cmdBus, "PlatformCreateUser", createCmd,
		func(ctx context.Context) (any, error) { return nil, ch.Handle(ctx, createCmd) })
	assertPermissionDenied(t, "create user", createErr)

	// The denied writes never ran — bob is untouched and mallory never existed.
	got, err := fx.Users.FindByID(ctx, victim.ID)
	if err != nil || got.Nickname != "bob" {
		t.Fatal("admin-denied platform writes must not have executed")
	}
	if m, _ := fx.Users.FindByNickname(ctx, "mallory"); m != nil {
		t.Fatal("a denied platform create must not have written a row")
	}
}

// The tenant write commands get the same treatment, for the same reason: each
// gates on a platform:* string, and a fat-fingered admin:* one would hand a
// tenant admin the power to delete tenants (or mint them) with every other test
// still green. Create-tenant is included because it is no longer CLIOnly — the
// bus's Authorize is now the only thing gating it on the HTTP path.
func TestPlatformTenantCommands_AdminDeniedAtBus(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_tenant_authz.db"))
	victim := fx.SeedTenant(t, "Ghost")
	cmdBus, _, _ := fx.NewBuses()

	adminCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{UserID: "a1", Role: "admin"})

	th := NewDeletePlatformTenantHandler(fx.PlatformTenants)
	deleteCmd := DeletePlatformTenantCommand{ID: victim.ID}
	_, delErr := testfx.ExecCommand(adminCtx, cmdBus, "PlatformDeleteTenant", deleteCmd,
		func(ctx context.Context) (any, error) { return nil, th.Handle(ctx, deleteCmd) })
	assertPermissionDenied(t, "delete tenant", delErr)

	bh := NewBulkDeletePlatformTenantsHandler(fx.PlatformTenants)
	bulkCmd := BulkDeletePlatformTenantsCommand{IDs: []string{victim.ID}}
	_, bulkErr := testfx.ExecCommand(adminCtx, cmdBus, "BulkDeletePlatformTenants", bulkCmd,
		func(ctx context.Context) (any, error) { return bh.Handle(ctx, bulkCmd) })
	assertPermissionDenied(t, "bulk delete tenants", bulkErr)

	// The tenant survived both denied deletes.
	if got, _ := fx.Tenants.FindByID(ctx, victim.ID); got == nil {
		t.Fatal("a denied tenant delete must not have executed")
	}
}

func assertPermissionDenied(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("admin must be denied the platform %s at the bus", label)
	}
	var pe *shared.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("platform %s: expected *shared.PermissionError, got %T: %v", label, err, err)
	}
}

// The out-of-band creation path mints a superadmin in the default tenant — the
// sanctioned way to add one (the admin API refuses the role).
func TestCreateSuperAdminHandler_CreatesSuperAdmin(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin.db"))

	h := NewCreateSuperAdminHandler(fx.Users, fx.Hasher)
	if err := h.Handle(ctx, CreateSuperAdminCommand{
		Nickname: "root",
		Password: "secret12",
		Email:    "root@example.com",
	}); err != nil {
		t.Fatalf("create superadmin: %v", err)
	}

	got, err := fx.Users.FindByNickname(ctx, "root")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("superadmin must be persisted")
	}
	if got.Role != "superadmin" {
		t.Fatalf("role: got %q want superadmin", got.Role)
	}
	if got.TenantID != shared.DefaultTenantID {
		t.Fatalf("superadmin must live in the default tenant, got %q", got.TenantID)
	}
	if err := fx.Hasher.Verify("secret12", got.PasswordHash); err != nil {
		t.Fatalf("password verify: %v", err)
	}
}

// F-031: superadmin creation must announce a UserCreated event, the same as
// CreateUser. The shared userwrite.Create body single-sources the announcement so
// it can't silently skip the event again (it used to).
func TestCreateSuperAdminHandler_EmitsUserCreatedEvent(t *testing.T) {
	ctx, collector := shared.ContextWithEventCollector(context.Background())
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_event.db"))

	h := NewCreateSuperAdminHandler(fx.Users, fx.Hasher)
	if err := h.Handle(ctx, CreateSuperAdminCommand{
		Nickname: "root",
		Password: "secret12",
		Email:    "root@example.com",
	}); err != nil {
		t.Fatalf("create superadmin: %v", err)
	}

	events := collector.Flush()
	if len(events) != 1 {
		t.Fatalf("events: got %d want 1", len(events))
	}
	ev, ok := events[0].(user.UserCreated)
	if !ok {
		t.Fatalf("expected UserCreated event, got %T", events[0])
	}
	if ev.Role != "superadmin" {
		t.Fatalf("event role: got %q want superadmin", ev.Role)
	}
}

func TestCreateSuperAdminHandler_RejectsDuplicateNickname(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_dup.db"))
	fx.SeedUserInTenant(t, "root", "user", shared.DefaultTenantID)

	h := NewCreateSuperAdminHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, CreateSuperAdminCommand{Nickname: "root", Password: "secret12"})
	if err == nil {
		t.Fatal("a duplicate nickname must be rejected")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "nickname" {
		t.Fatalf("expected nickname ValidationError, got %T: %v", err, err)
	}
}

func TestCreateSuperAdminHandler_RejectsShortPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_pw.db"))

	h := NewCreateSuperAdminHandler(fx.Users, fx.Hasher)
	if err := h.Handle(ctx, CreateSuperAdminCommand{Nickname: "root", Password: "short"}); err == nil {
		t.Fatal("a too-short password must be rejected")
	}
}

// F-011 not-found branch: a platform edit/delete of a NONEXISTENT id must fail
// with a field-keyed ValidationError (400), never silently no-op — the same
// contract the admin-plane handlers pin. Guards the target==nil branch that
// FindByIDAcrossTenants' (nil, nil) not-found contract feeds.
func TestUpdatePlatformUser_NotFound(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_update_nf.db"))

	h := NewUpdatePlatformUserHandler(fx.PlatformUsers, fx.Hasher)
	err := h.Handle(ctx, UpdatePlatformUserCommand{
		ID:       "0193b1e0-0000-7000-8000-000000000000",
		Nickname: "ghost",
		Email:    "ghost@example.com",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "id" {
		t.Fatalf(
			"update of a nonexistent id must return ValidationError{Field:\"id\"}, got %v",
			err,
		)
	}
}

func TestDeletePlatformUser_NotFound(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_delete_nf.db"))

	callerCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: "caller-super", Role: "superadmin",
	})

	h := NewDeletePlatformUserHandler(fx.PlatformUsers)
	err := h.Handle(
		callerCtx,
		DeletePlatformUserCommand{ID: "0193b1e0-0000-7000-8000-000000000001"},
	)

	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "id" {
		t.Fatalf(
			"delete of a nonexistent id must return ValidationError{Field:\"id\"}, got %v",
			err,
		)
	}
}
