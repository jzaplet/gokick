package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/infrastructure/config"

	"github.com/golang-jwt/jwt/v5"
)

type JwtService struct {
	secret            []byte
	accessExpiration  time.Duration
	refreshExpiration time.Duration
}

// minJWTSecretLen is the floor for an HS256 secret. RFC 7518 §3.2 recommends
// the key be at least as long as the HMAC output (256 bits = 32 bytes).
const minJWTSecretLen = 32

func NewJwtService(cfg *config.Config) (*JwtService, error) {
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("security: APP_JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < minJWTSecretLen {
		return nil, fmt.Errorf(
			"security: APP_JWT_SECRET must be at least %d characters",
			minJWTSecretLen,
		)
	}
	return &JwtService{
		secret:            []byte(cfg.JWTSecret),
		accessExpiration:  cfg.JWTAccessExpiration,
		refreshExpiration: cfg.JWTRefreshExpiration,
	}, nil
}

func (s *JwtService) GenerateAccessToken(claims *shared.AuthClaims) (string, time.Duration, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      claims.UserID,
		"role":     claims.Role,
		"nickname": claims.Nickname,
		"email":    claims.Email,
		"tenant":   claims.TenantID,
		"lang":     claims.Lang,
		"iat":      now.Unix(),
		"exp":      now.Add(s.accessExpiration).Unix(),
	})

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", 0, err
	}
	return signed, s.accessExpiration, nil
}

func (s *JwtService) ValidateAccessToken(tokenString string) (*shared.AuthClaims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("security: unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, &shared.AuthError{Key: msgkey.AuthTokenInvalid}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, &shared.AuthError{Key: msgkey.AuthTokenClaimsInvalid}
	}

	userID := claimString(claims, "sub")
	role := claimString(claims, "role")
	if userID == "" || role == "" {
		// Fail closed: a validly-signed token whose `sub` or `role` claim is
		// absent/empty must be rejected, not silently downgraded to baseline
		// (empty-role) permissions. GenerateAccessToken always sets both, so no
		// legitimate token is affected. Nickname/email/tenant stay tolerant.
		return nil, &shared.AuthError{Key: msgkey.AuthTokenClaimsInvalid}
	}

	return &shared.AuthClaims{
		UserID:   userID,
		Role:     role,
		Nickname: claimString(claims, "nickname"),
		Email:    claimString(claims, "email"),
		TenantID: claimString(claims, "tenant"),
		Lang:     claimString(claims, "lang"),
	}, nil
}

func (s *JwtService) GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error) {
	raw = rand.Text()
	hash = HashToken(raw)
	expiresAt = time.Now().Add(s.refreshExpiration)
	return raw, hash, expiresAt, nil
}

func (s *JwtService) RefreshExpiration() time.Duration {
	return s.refreshExpiration
}

func (*JwtService) HashRefreshToken(raw string) string {
	return HashToken(raw)
}

func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func claimString(claims jwt.MapClaims, key string) string {
	v, _ := claims[key].(string)
	return v
}
