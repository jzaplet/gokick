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

	u := user.NewUser(nickname, hash, email, role)
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
