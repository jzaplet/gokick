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
	users     user.Repository
	tokens    token.TokenRepository
	password  shared.PasswordHasher
	jwt       shared.JwtService
	dummyHash string
}

// dummyPasswordPlaceholder seeds the hash we Verify against when the
// nickname doesn't exist. It must never match a real password — the
// value here is irrelevant, only the cost-12 bcrypt comparison time is.
const dummyPasswordPlaceholder = "TIMING-PLACEHOLDER-NOT-A-REAL-PASSWORD"

func NewLoginHandler(
	users user.Repository,
	tokens token.TokenRepository,
	password shared.PasswordHasher,
	jwt shared.JwtService,
) *LoginHandler {
	// Pay the bcrypt cost once at startup so the "user not found" branch
	// can compare against a real hash and match the timing of "user
	// found, wrong password". Without this, response time leaks whether
	// a nickname exists.
	dummy, err := password.Hash(dummyPasswordPlaceholder)
	if err != nil {
		// Hashing a fixed string can only fail on a misconfigured
		// hasher — fall back to an empty string so Verify always
		// fails uniformly; both branches still pay the Compare cost.
		dummy = ""
	}
	return &LoginHandler{
		users:     users,
		tokens:    tokens,
		password:  password,
		jwt:       jwt,
		dummyHash: dummy,
	}
}

func (h *LoginHandler) Handle(ctx context.Context, cmd LoginCommand) (LoginResult, error) {
	u, err := h.users.FindByNickname(ctx, cmd.Nickname)
	if err != nil {
		return LoginResult{}, err
	}

	// Always call Verify so an attacker timing the response can't tell
	// whether the nickname existed: the user-not-found branch verifies
	// against a startup-precomputed dummy hash, matching the bcrypt
	// cost of the user-found branch. The boolean below collapses both
	// outcomes into one neutral AuthError after the work is done.
	hash := h.dummyHash
	if u != nil {
		hash = u.PasswordHash
	}
	verifyErr := h.password.Verify(cmd.Password, hash)

	if u == nil || verifyErr != nil {
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
