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

// The point of the platform create: the superadmin CHOOSES the tenant. The admin
// twin inherits it from ctx, so this is the one behaviour a copy of that handler
// would get wrong.
func TestCreatePlatformUser_CreatesInTheChosenTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_chosen.db"))

	target := fx.SeedTenant(t, "Beta")

	h := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob@example.com",
		Role:     "admin",
		TenantID: target.ID,
	})
	if err != nil {
		t.Fatalf("create in a chosen tenant must land: %v", err)
	}

	got, err := fx.Users.FindByNickname(ctx, "bob")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the user must exist")
	}
	if got.TenantID != target.ID {
		t.Fatalf("user must land in the CHOSEN tenant %q, got %q", target.ID, got.TenantID)
	}
	if got.Role != "admin" {
		t.Fatalf("role must persist, got %q", got.Role)
	}
}

// An unknown tenant owes the operator a 400 against the field, not the 500 an FK
// violation would produce (users.tenant_id REFERENCES tenants(id)).
func TestCreatePlatformUser_UnknownTenantIsAFieldError(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_badtenant.db"))

	h := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob@example.com",
		Role:     "user",
		TenantID: "01920000-0000-7000-8000-000000000000",
	})
	if err == nil {
		t.Fatal("an unknown tenant must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "tenant_id" {
		t.Fatalf("expected *shared.ValidationError{Field:\"tenant_id\"}, got %T: %v", err, err)
	}

	if got, _ := fx.Users.FindByNickname(ctx, "bob"); got != nil {
		t.Fatal("no user may be created when the tenant is unknown")
	}
}

// tenant_id is required in BOTH modes, and the table is the assertion: an earlier
// cut mirrored shared.RequireTenant and quietly defaulted to the default tenant
// when multitenancy was off. The form marks the field required, so that fallback
// made the UI lie — leave the picker blank and the user lands in the default
// tenant anyway, exactly as if you had chosen it.
//
// The picker always has at least one option (the default tenant is created by
// migration), so an empty tenant_id is never "there was nothing to choose". It is
// a field the caller could see and skipped, and it earns a 400 against that field
// — in either mode, which is why the flag is gone from the handler entirely.
func TestCreatePlatformUser_RequiresATenantInEitherMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		fx   func(*testing.T, string) *testfx.Fixture
	}{
		{"multitenant", testfx.NewMultitenant},
		{"single-tenant", testfx.New},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			fx := tc.fx(t, filepath.Join(t.TempDir(), "pcreate_notenant.db"))

			h := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
			err := h.Handle(ctx, CreatePlatformUserCommand{
				Nickname: "bob",
				Password: "correct horse battery staple",
				Email:    "bob@example.com",
				Role:     "user",
			})
			if err == nil {
				t.Fatal("a create with no tenant must be refused, not silently defaulted")
			}
			var ve *shared.ValidationError
			if !errors.As(err, &ve) || ve.Field != "tenant_id" {
				t.Fatalf("expected *shared.ValidationError{Field:\"tenant_id\"}, got %T: %v",
					err, err)
			}

			if got, _ := fx.Users.FindByNickname(ctx, "bob"); got != nil {
				t.Fatal("no user may be born tenant-less")
			}
		})
	}
}

// Nobody mints a superadmin over HTTP — not even a superadmin. The CLI and the
// seeder are the only paths, by design.
func TestCreatePlatformUser_RefusesTheSuperadminRole(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pcreate_super.db"))

	h := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "root2",
		Password: "correct horse battery staple",
		Email:    "r@x.com",
		Role:     "superadmin",
	})
	if err == nil {
		t.Fatal("creating a superadmin through the API must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected *shared.ValidationError{Field:\"role\"}, got %T: %v", err, err)
	}

	if got, _ := fx.Users.FindByNickname(ctx, "root2"); got != nil {
		t.Fatal("no superadmin may be created through the API")
	}
}

// THE regression: a create dispatched through the real bus, which is the only way
// an active tenant reaches ctx.
//
// Every test above calls Handle with a bare context, so no tenant is active and
// Save's AssertTenantScope has nothing to compare against — it waves the write
// through. In production TenantMiddleware resolves the superadmin's own tenant
// (the default one) into ctx, the row names a DIFFERENT tenant, and the guard
// rejects it: a 500 on a perfectly valid form. Only a bus dispatch shows that,
// which is why the handler-level tests all passed while the feature was broken.
func TestCreatePlatformUser_ThroughTheBus_WritesIntoAnotherTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_bus.db"))
	cmdBus, _, _ := fx.NewBuses()

	target := fx.SeedTenant(t, "Stark")

	// What the HTTP stack really hands the handler: superadmin claims, and (via
	// TenantMiddleware) the default tenant as the active scope.
	superCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID:   "s1",
		Role:     "superadmin",
		TenantID: shared.DefaultTenantID,
	})

	h := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
	cmd := CreatePlatformUserCommand{
		Nickname: "tony",
		Password: "correct horse battery staple",
		Email:    "tony@example.com",
		Role:     "admin",
		TenantID: target.ID,
	}
	_, err := testfx.ExecCommand(superCtx, cmdBus, "PlatformCreateUser", cmd,
		func(ctx context.Context) (any, error) { return nil, h.Handle(ctx, cmd) })
	if err != nil {
		t.Fatalf("a superadmin must create into another tenant through the bus: %v", err)
	}

	got, err := fx.Users.FindByNickname(ctx, "tony")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the user must exist")
	}
	if got.TenantID != target.ID {
		t.Fatalf("user must land in the chosen tenant %q, not the actor's own — got %q",
			target.ID, got.TenantID)
	}
}

// The UserCreated event must carry the tenant the user actually landed in — the
// CHOSEN one, not the actor's.
//
// No subscriber exists yet (provideEventHandlers is empty but for a commented-out
// `{Event: "user.created", Handler: welcomeMailer.Handle}`), which is exactly why
// this is pinned now. Events dispatch after commit in the COMMAND's context, whose
// active tenant is the superadmin's own — so a future handler reading the tenant
// from ctx would be right on every pre-existing create path and wrong only here,
// silently, past the point any request error could surface it. The field is what
// makes ctx the wrong place to look.
func TestCreatePlatformUser_EventCarriesTheChosenTenant(t *testing.T) {
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_event.db"))

	target := fx.SeedTenant(t, "Stark")

	// The actor's active tenant is the default one — what TenantMiddleware sets
	// for a superadmin. If the event took its tenant from ctx it would say this.
	ctx, collector := shared.ContextWithEventCollector(context.Background())
	ctx = shared.ContextWithTenantID(ctx, shared.DefaultTenantID)

	h := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "tony",
		Password: "correct horse battery staple",
		Email:    "tony@example.com",
		Role:     "admin",
		TenantID: target.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	events := collector.Flush()
	if len(events) != 1 {
		t.Fatalf("events: got %d want 1", len(events))
	}
	ev, ok := events[0].(user.UserCreated)
	if !ok {
		t.Fatalf("expected UserCreated, got %T", events[0])
	}
	if ev.TenantID != target.ID {
		t.Fatalf("event must carry the CHOSEN tenant %q, got %q — a subscriber reading "+
			"the tenant from ctx would get the actor's (%q)",
			target.ID, ev.TenantID, shared.DefaultTenantID)
	}
}

// The other half of the same coin: the ADMIN create must STILL be refused when it
// names a tenant other than the active one. SaveAcrossTenants opened a hole in the
// scope guard; this proves the hole is confined to the platform plane and did not
// widen Save for everyone.
func TestCreateUser_AdminPlane_StillCannotCrossTenants(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "acreate_cross.db"))

	other := fx.SeedTenant(t, "Victim Corp")

	// An admin acting inside their own tenant, with a row aimed at another one.
	adminCtx := shared.ContextWithTenantID(ctx, shared.DefaultTenantID)
	u := user.NewUser("mallory", "hash", "", user.RoleAdmin, other.ID)

	if err := fx.Users.Save(adminCtx, u); err == nil {
		t.Fatal("Save must still refuse a row aimed outside the active tenant")
	}

	// ...and the platform port is the ONLY way through.
	if err := fx.PlatformUsers.SaveAcrossTenants(adminCtx, u); err != nil {
		t.Fatalf("SaveAcrossTenants is the sanctioned exception: %v", err)
	}
}

// A nickname is globally unique (users.nickname UNIQUE), so the collision check
// must reach ACROSS tenants — a platform create into tenant B must still trip on
// a nickname held in tenant A. This is why the shared body's FindByNickname is a
// deliberately unscoped identity lookup.
func TestCreatePlatformUser_NicknameCollidesAcrossTenants(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_collide.db"))

	tenantA := fx.SeedTenant(t, "Alpha")
	tenantB := fx.SeedTenant(t, "Beta")
	fx.SeedUserInTenant(t, "bob", "user", tenantA.ID)

	h := NewCreatePlatformUserHandler(fx.PlatformUsers, fx.Tenants, fx.Hasher)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob2@example.com",
		Role:     "user",
		TenantID: tenantB.ID,
	})
	if err == nil {
		t.Fatal("a nickname taken in ANOTHER tenant must still collide (it is globally unique)")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "nickname" {
		t.Fatalf("expected *shared.ValidationError{Field:\"nickname\"}, got %T: %v", err, err)
	}
}
