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

func TestCreateUserHandler_Success(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_success.db"))
	ctx, collector := shared.ContextWithEventCollector(context.Background())

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, CreateUserCommand{
		Nickname: "bob",
		Password: "secret12",
		Email:    "bob@example.com",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// User exists in DB
	saved, err := fx.Users.FindByNickname(ctx, "bob")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if saved == nil {
		t.Fatal("expected user persisted in DB")
	}
	if saved.Email != "bob@example.com" {
		t.Fatalf("email: got %s want bob@example.com", saved.Email)
	}
	if saved.Role != "user" {
		t.Fatalf("role: got %s want user", saved.Role)
	}
	if err := fx.Hasher.Verify("secret12", saved.PasswordHash); err != nil {
		t.Fatalf("password verify: %v", err)
	}

	// Event collected
	events := collector.Flush()
	if len(events) != 1 {
		t.Fatalf("events: got %d want 1", len(events))
	}
	ev, ok := events[0].(user.UserCreated)
	if !ok {
		t.Fatalf("expected UserCreated event, got %T", events[0])
	}
	if ev.UserID != saved.ID {
		t.Fatalf("event UserID: got %s want %s", ev.UserID, saved.ID)
	}
}

func TestCreateUserHandler_DuplicateNickname(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_dup.db"))
	fx.SeedUser(t, "alice", "existing", "user")

	ctx, collector := shared.ContextWithEventCollector(context.Background())
	h := NewCreateUserHandler(fx.Users, fx.Hasher)

	err := h.Handle(ctx, CreateUserCommand{
		Nickname: "alice",
		Password: "secret12",
		Email:    "alice2@example.com",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "nickname" {
		t.Fatalf("expected field=nickname, got %s", ve.Field)
	}
	if len(collector.Flush()) != 0 {
		t.Fatal("event must not be collected when save is skipped")
	}
}

func TestCreateUserHandler_EmptyNickname(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_empty_nick.db"))

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(context.Background(), CreateUserCommand{
		Nickname: "",
		Password: "secret12",
		Email:    "x@y.com",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}
	if ve.Field != "nickname" {
		t.Fatalf("expected field=nickname, got %s", ve.Field)
	}
}

func TestCreateUserHandler_InvalidRole(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_invalid_role.db"))

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(context.Background(), CreateUserCommand{
		Nickname: "bob",
		Password: "secret12",
		Email:    "bob@example.com",
		Role:     "superhero",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}
	if ve.Field != "role" {
		t.Fatalf("expected field=role, got %s", ve.Field)
	}
}

func TestCreateUserHandler_EmptyPassword(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_empty_pwd.db"))

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(context.Background(), CreateUserCommand{
		Nickname: "bob",
		Password: "",
		Email:    "bob@example.com",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}
	if ve.Field != "password" {
		t.Fatalf("expected field=password, got %s", ve.Field)
	}
}

func TestCreateUserHandler_OptionalEmail(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_optional_email.db"))

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, CreateUserCommand{
		Nickname: "bob",
		Password: "secret12",
		Email:    "",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("expected success with empty email, got %v", err)
	}

	saved, err := fx.Users.FindByNickname(ctx, "bob")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if saved == nil {
		t.Fatal("expected user persisted in DB")
	}
	if saved.Email != "" {
		t.Fatalf("email: got %q want empty", saved.Email)
	}
}

func TestCreateUserHandler_InvalidEmail(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_invalid_email.db"))

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(context.Background(), CreateUserCommand{
		Nickname: "bob",
		Password: "secret12",
		Email:    "no-at-sign",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}
	if ve.Field != "email" {
		t.Fatalf("expected field=email, got %s", ve.Field)
	}
}

func TestCreateUserCommand_RequiredPermission(t *testing.T) {
	if got := (CreateUserCommand{}).RequiredPermission(); got != "admin:users:create" {
		t.Fatalf("expected admin:users:create, got %q", got)
	}
}

// A user created by an admin must land in the ADMIN's tenant — the caller's
// resolved tenant carried in ctx (as the bus's TenantMiddleware supplies it) —
// not always the default. Without the stamp, every admin-created user would land
// in the default tenant, a cross-tenant isolation gap once provisioning exists.
func TestCreateUserHandler_StampsCallerTenant(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_caller_tenant.db"))
	tenantB := fx.SeedTenant(t, "Beta")

	ctx, _ := shared.ContextWithEventCollector(context.Background())
	ctx = shared.ContextWithTenantID(ctx, tenantB.ID)

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	if err := h.Handle(ctx, CreateUserCommand{
		Nickname: "bob",
		Password: "secret12",
		Email:    "bob@example.com",
		Role:     "user",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	saved, err := fx.Users.FindByNickname(ctx, "bob")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if saved == nil {
		t.Fatal("expected user persisted")
	}
	if saved.TenantID != tenantB.ID {
		t.Fatalf(
			"created user must land in the caller's tenant %q, got %q",
			tenantB.ID,
			saved.TenantID,
		)
	}
}

// Invoked outside the bus (the CLI create-user), there is no tenant in ctx; the
// user must fall back to the default tenant, not an empty (FK-violating) value.
func TestCreateUserHandler_DefaultsTenantWithoutContext(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_default_tenant.db"))

	ctx, _ := shared.ContextWithEventCollector(context.Background())

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	if err := h.Handle(ctx, CreateUserCommand{
		Nickname: "cli",
		Password: "secret12",
		Email:    "cli@example.com",
		Role:     "admin",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	saved, err := fx.Users.FindByNickname(ctx, "cli")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if saved == nil || saved.TenantID != shared.DefaultTenantID {
		t.Fatalf("CLI-created user must default to the default tenant, got %+v", saved)
	}
}
