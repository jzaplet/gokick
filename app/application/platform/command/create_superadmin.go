package command

import (
	"context"

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

	existing, err := h.users.FindByNickname(ctx, string(nickname))
	if err != nil {
		return err
	}
	if existing != nil {
		return &shared.ValidationError{
			Field:   "nickname",
			Message: "user with this nickname already exists",
		}
	}

	hash, err := h.password.Hash(string(password))
	if err != nil {
		return err
	}

	// A superadmin is a cross-tenant identity; home it in the default tenant
	// explicitly (its own tenant is immaterial to the platform plane).
	u := user.NewUser(nickname, hash, email, user.RoleSuperAdmin, shared.DefaultTenantID)
	if err := h.users.Save(ctx, u); err != nil {
		return err
	}

	// Mirrors CreateUserHandler's audit. Outside the bus (no AuditMiddleware) the
	// collector is a throwaway; through the SystemCommandBus it persists the trail
	// of minting a cross-tenant account.
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.created",
		TargetType: "user",
		TargetID:   u.ID,
		Metadata:   map[string]any{"role": u.Role},
	})

	return nil
}
