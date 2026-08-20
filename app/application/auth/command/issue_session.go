package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
)

// issueSession mints an access+refresh token pair for u, persists the refresh
// token, and returns the populated IssuedSession. It is the single source of
// truth for the AuthClaims shape and the RefreshToken construction, shared by
// the login and refresh handlers (F-025) — a new claim or a token-TTL policy
// change lands in one place instead of drifting between the two issuance paths
// (the near-miss the finding names: a claim silently present on one path only).
//
// The helper stays PURE issuance: it does NOT audit, mark the presented token
// used, or reset the failed-login counter. Those are caller-specific steps that
// run AFTER Save (login → auth.login.succeeded; refresh → MarkUsed the old
// token), and Save is this helper's final write, so the Save-before-those-steps
// ordering both handlers rely on is preserved by calling them after it returns.
func issueSession(
	ctx context.Context,
	jwt shared.TokenService,
	tokens token.Repository,
	u *user.User,
) (IssuedSession, error) {
	accessToken, accessExpiresIn, err := jwt.GenerateAccessToken(&shared.AuthClaims{
		UserID:   u.ID,
		Role:     u.Role,
		Nickname: u.Nickname,
		Email:    u.Email,
		TenantID: u.TenantID,
		Lang:     u.PreferredLang(),
	})
	if err != nil {
		return IssuedSession{}, err
	}

	rawRefresh, hash, expiresAt, err := jwt.GenerateRefreshToken()
	if err != nil {
		return IssuedSession{}, err
	}

	rt := token.NewRefreshToken(u.ID, hash, expiresAt)
	if err := tokens.Save(ctx, rt); err != nil {
		return IssuedSession{}, err
	}

	return IssuedSession{
		User:             *u,
		AccessToken:      accessToken,
		AccessExpiresIn:  accessExpiresIn,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: expiresAt,
	}, nil
}
