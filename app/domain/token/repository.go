package token

import "context"

type TokenRepository interface {
	Save(ctx context.Context, token *RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
	// MarkUsed atomically flips a token's used_at from NULL to now. Returns
	// true on success. False means the token was already used — caller must
	// treat this as theft (concurrent rotation attempt with the same raw
	// token) and revoke the user's session.
	MarkUsed(ctx context.Context, hash string) (bool, error)
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context) error
}
