package command

import (
	"context"

	"gokick/app/application/userwrite"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// CreateSuperAdminCommand creates a platform superadmin account. This is the
// sanctioned OUT-OF-BAND creation path (trusted operator / CLI): the normal
// admin user commands REFUSE to assign the superadmin role so a tenant admin
// can't self-escalate, so minting a superadmin happens here instead. The
// account lives in the default tenant (NewUser stamps it) — platform queries are
// cross-tenant, so its own tenant is immaterial.
type CreateSuperAdminCommand struct {
	Nickname string
	Password string
	Email    string
}

func (CreateSuperAdminCommand) RequiredPermission() string { return "platform:users:create" }

// CLIOnly: create-superadmin runs only via the CLI/SystemCommandBus (no HTTP
// route), so its permission stays out of the FE-facing registry. See shared.CLIOnly.
func (CreateSuperAdminCommand) CLIOnly() {}

type CreateSuperAdminHandler struct {
	users    user.Repository
	password shared.PasswordHasher
}

func NewCreateSuperAdminHandler(
	users user.Repository,
	password shared.PasswordHasher,
) *CreateSuperAdminHandler {
	return &CreateSuperAdminHandler{users: users, password: password}
}

func (h *CreateSuperAdminHandler) Handle(ctx context.Context, cmd CreateSuperAdminCommand) error {
	nickname, err := user.NewNickname(cmd.Nickname)
	if err != nil {
		return err
	}

	password, err := user.NewPassword(cmd.Password)
	if err != nil {
		return err
	}

	email, err := user.NewEmail(cmd.Email)
	if err != nil {
		return err
	}

	// A superadmin is a cross-tenant identity; home it in the default tenant
	// explicitly (its own tenant is immaterial to the platform plane). Uniqueness +
	// hash + persist + announce go through the shared create body — the same one
	// CreateUser uses — so the user.created audit AND the UserCreated event fire for
	// a superadmin too (previously the event was silently skipped; F-031).
	_, err = userwrite.Create(
		ctx,
		h.users,
		h.password,
		nickname,
		password,
		email,
		user.RoleSuperAdmin,
		shared.DefaultTenantID,
	)
	return err
}
