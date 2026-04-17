package command

import (
	"context"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"

	"github.com/google/uuid"
)

type LoginCommand struct {
	Nickname string
	Password string
}

func (LoginCommand) SkipPermissionCheck() {}

type LoginResult struct {
	User             user.User
	AccessToken      string
	AccessExpiresIn  time.Duration
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type LoginHandler struct {
	users    user.Repository
	tokens   token.TokenRepository
	password shared.PasswordHasher
	jwt      shared.JwtService
}

func NewLoginHandler(
	users user.Repository,
	tokens token.TokenRepository,
	password shared.PasswordHasher,
	jwt shared.JwtService,
) *LoginHandler {
	return &LoginHandler{
		users:    users,
		tokens:   tokens,
		password: password,
		jwt:      jwt,
	}
}

func (h *LoginHandler) Handle(ctx context.Context, cmd LoginCommand) (LoginResult, error) {
	u, err := h.users.FindByNickname(ctx, cmd.Nickname)
	if err != nil {
		return LoginResult{}, err
	}
	if u == nil {
		return LoginResult{}, &shared.AuthError{Message: "invalid credentials"}
	}

	if err := h.password.Verify(cmd.Password, u.PasswordHash); err != nil {
		return LoginResult{}, &shared.AuthError{Message: "invalid credentials"}
	}

	accessToken, accessExpiresIn, err := h.jwt.GenerateAccessToken(&shared.AuthClaims{
		UserID:   u.ID,
		Role:     u.Role,
		Nickname: u.Nickname,
	})
	if err != nil {
		return LoginResult{}, err
	}

	rawRefresh, hash, expiresAt, err := h.jwt.GenerateRefreshToken()
	if err != nil {
		return LoginResult{}, err
	}

	rt := &token.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	if err := h.tokens.Save(ctx, rt); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User:             *u,
		AccessToken:      accessToken,
		AccessExpiresIn:  accessExpiresIn,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: expiresAt,
	}, nil
}
