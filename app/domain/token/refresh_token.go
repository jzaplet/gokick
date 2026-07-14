package token

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        string     `db:"id"`
	UserID    string     `db:"user_id"`
	TokenHash string     `db:"token_hash"`
	ExpiresAt time.Time  `db:"expires_at"`
	CreatedAt time.Time  `db:"created_at"`
	UsedAt    *time.Time `db:"used_at"`
}

// NewRefreshToken constructs a fresh, unused refresh token: a generated id, the
// given user / hash / expiry, CreatedAt=now and UsedAt nil. Entity identity and
// timestamps belong in the domain, as user/run/tenant already do in their New*
// factories — so the ID generator is one place to change, not three.
func NewRefreshToken(userID, tokenHash string, expiresAt time.Time) *RefreshToken {
	return &RefreshToken{
		ID:        uuid.New().String(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
}
