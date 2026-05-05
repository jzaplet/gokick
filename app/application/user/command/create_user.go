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
	events   *shared.EventCollector
}

func NewCreateUserHandler(
	users user.Repository,
	password shared.PasswordHasher,
	events *shared.EventCollector,
) *CreateUserHandler {
	return &CreateUserHandler{
		users:    users,
		password: password,
		events:   events,
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
			Message: "uživatel s tímto nickname už existuje",
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

	h.events.Collect(user.UserCreated{
		UserID:    u.ID,
		Nickname:  u.Nickname,
		Email:     u.Email,
		Role:      u.Role,
		Timestamp: time.Now(),
	})

	return nil
}
