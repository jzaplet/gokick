package command

import (
	"context"
	"time"

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
	users    user.Repository
	password shared.PasswordHasher
}

func NewCreateUserHandler(
	users user.Repository,
	password shared.PasswordHasher,
) *CreateUserHandler {
	return &CreateUserHandler{
		users:    users,
		password: password,
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

	// Resolve the tenant the user lands in: the bus's TenantMiddleware always puts
	// the caller's resolved tenant in ctx (HTTP), so an admin creates users in
	// their OWN tenant; an empty value means the handler ran outside the bus (the
	// CLI create-user), where the single-tenant default is correct. NewUser
	// requires the tenant explicitly (born scoped). Mirrors the dispatcher guard.
	tenantID := shared.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = shared.DefaultTenantID
	}
	u := user.NewUser(nickname, hash, email, role, tenantID)

	if err := h.users.Save(ctx, u); err != nil {
		return err
	}

	shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{
		UserID:    u.ID,
		Nickname:  u.Nickname,
		Email:     u.Email,
		Role:      u.Role,
		Timestamp: time.Now(),
	})

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.created",
		TargetType: "user",
		TargetID:   u.ID,
		Metadata:   map[string]any{"role": u.Role},
	})

	return nil
}
