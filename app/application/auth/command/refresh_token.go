package command

import (
	"context"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"

	"github.com/google/uuid"
)

type RefreshTokenCommand struct {
	RawToken string
}

func (RefreshTokenCommand) SkipPermissionCheck() {}

type RefreshTokenHandler struct {
	users  user.Repository
	tokens token.TokenRepository
	jwt    shared.JwtService
}

func NewRefreshTokenHandler(
	users user.Repository,
	tokens token.TokenRepository,
	jwt shared.JwtService,
) *RefreshTokenHandler {
	return &RefreshTokenHandler{
		users:  users,
		tokens: tokens,
		jwt:    jwt,
	}
}

func (h *RefreshTokenHandler) Handle(ctx context.Context, cmd RefreshTokenCommand) (LoginResult, error) {
	hash := h.jwt.HashRefreshToken(cmd.RawToken)

	existing, err := h.tokens.FindByHash(ctx, hash)
	if err != nil {
		return LoginResult{}, err
	}
	if existing == nil {
		return LoginResult{}, &shared.AuthError{Message: "invalid refresh token"}
	}

	if time.Now().After(existing.ExpiresAt) {
		// Expired — clean up so it can't be used again.
		_ = h.tokens.DeleteByUserID(ctx, existing.UserID)
		return LoginResult{}, &shared.AuthError{Message: "refresh token expired"}
	}

	u, err := h.users.FindByID(ctx, existing.UserID)
	if err != nil {
		return LoginResult{}, &shared.AuthError{Message: "user no longer exists"}
	}

	// Rotate: delete the used token, then issue a fresh pair.
	if err := h.tokens.DeleteByUserID(ctx, u.ID); err != nil {
		return LoginResult{}, err
	}

	accessToken, accessExpiresIn, err := h.jwt.GenerateAccessToken(&shared.AuthClaims{
		UserID:   u.ID,
		Role:     u.Role,
		Nickname: u.Nickname,
	})
	if err != nil {
		return LoginResult{}, err
	}

	rawRefresh, newHash, expiresAt, err := h.jwt.GenerateRefreshToken()
	if err != nil {
		return LoginResult{}, err
	}

	rt := &token.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		TokenHash: newHash,
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
