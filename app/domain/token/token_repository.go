package token

import "context"

type TokenRepository interface {
	Save(ctx context.Context, token *RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context) error
}
