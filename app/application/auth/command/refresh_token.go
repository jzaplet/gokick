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

// SkipTransaction keeps RefreshToken out of the bus tx — but for a different
// reason than LoginCommand (which is about a raw-pool self-deadlock). The theft
// path calls tokens.DeleteByUserID and then returns an AuthError. DeleteByUserID
// is tx-aware (r.Conn), so under a bus tx that AuthError would roll the deletion
// BACK and defeat the force-logout the theft response depends on. Running
// outside the tx lets the cleanup auto-commit and persist. The happy-path
// rotation (FindByHash → MarkUsed → Save) is consequently non-atomic, which is
// safe: a failed Save after MarkUsed just funnels the next attempt into theft
// detection on retry.
func (RefreshTokenCommand) SkipTransaction() {}

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

func (h *RefreshTokenHandler) Handle(
	ctx context.Context,
	cmd RefreshTokenCommand,
) (LoginResult, error) {
	hash := h.jwt.HashRefreshToken(cmd.RawToken)

	existing, err := h.tokens.FindByHash(ctx, hash)
	if err != nil {
		return LoginResult{}, err
	}
	if existing == nil {
		return LoginResult{}, &shared.AuthError{Message: "invalid refresh token"}
	}

	audit := shared.AuditCollectorFromContext(ctx)

	// Theft detection: a token that was already rotated is being presented again.
	// Assume credentials are compromised and log the user out on all devices.
	if existing.UsedAt != nil {
		_ = h.tokens.DeleteByUserID(ctx, existing.UserID)
		audit.Record(shared.AuditEvent{
			Action:     "auth.token.theft_detected",
			TargetType: "user",
			TargetID:   existing.UserID,
			Metadata:   map[string]any{"reason": "reused_after_rotation"},
		})
		return LoginResult{}, &shared.AuthError{Message: "refresh token reuse detected"}
	}

	if time.Now().After(existing.ExpiresAt) {
		_ = h.tokens.DeleteByUserID(ctx, existing.UserID)
		return LoginResult{}, &shared.AuthError{Message: "refresh token expired"}
	}

	u, err := h.users.FindByID(ctx, existing.UserID)
	if err != nil {
		return LoginResult{}, &shared.AuthError{Message: "user no longer exists"}
	}

	// Rotate: atomically mark the current token as used. If the update
	// touched 0 rows, a concurrent request rotated it first — treat that
	// as theft (the raw token is now in two places) and revoke everything.
	marked, err := h.tokens.MarkUsed(ctx, hash)
	if err != nil {
		return LoginResult{}, err
	}
	if !marked {
		_ = h.tokens.DeleteByUserID(ctx, existing.UserID)
		audit.Record(shared.AuditEvent{
			Action:     "auth.token.theft_detected",
			TargetType: "user",
			TargetID:   existing.UserID,
			Metadata:   map[string]any{"reason": "concurrent_rotation_race"},
		})
		return LoginResult{}, &shared.AuthError{Message: "refresh token reuse detected"}
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
