package command

import (
	"context"

	"gokick/app/application/userwrite"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

type CreateUserCommand struct {
	Nickname string
	Password string
	Email    string
	Role     string
}

func (CreateUserCommand) RequiredPermission() string { return "admin:users:create" }

type CreateUserHandler struct {
	users       user.Repository
	password    shared.PasswordHasher
	multitenant shared.Multitenancy
}

func NewCreateUserHandler(
	users user.Repository,
	password shared.PasswordHasher,
	multitenant shared.Multitenancy,
) *CreateUserHandler {
	return &CreateUserHandler{
		users:       users,
		password:    password,
		multitenant: multitenant,
	}
}

func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
	nickname, err := user.NewNickname(cmd.Nickname)
	if err != nil {
		return err
	}

	role, err := user.NewRole(cmd.Role)
	if err != nil {
		return err
	}
	if role.IsSuperAdmin() {
		return &shared.ValidationError{
			Field:   "role",
			Message: "cannot assign the superadmin role",
		}
	}

	password, err := user.NewPassword(cmd.Password)
	if err != nil {
		return err
	}

	email, err := user.NewEmail(cmd.Email)
	if err != nil {
		return err
	}

	// Resolve the tenant the user lands in: the bus's TenantMiddleware always puts
	// the caller's resolved tenant in ctx (HTTP), so an admin creates users in their
	// OWN tenant. An empty value means the handler ran outside the bus (the CLI
	// create-user) — RequireTenant resolves it fail-closed: the single-tenant default
	// when off, an error when multitenancy is on (a user must not be silently born in
	// the default tenant). NewUser requires the tenant explicitly (born scoped).
	tenantID, err := shared.RequireTenant(shared.TenantIDFromContext(ctx), h.multitenant)
	if err != nil {
		return err
	}

	// Uniqueness + hash + persist + announce (user.created audit & UserCreated event)
	// are the shared create body — same as CreateSuperAdmin, so the event can't drift.
	_, err = userwrite.Create(ctx, h.users, h.password, nickname, password, email, role, tenantID)
	return err
}
