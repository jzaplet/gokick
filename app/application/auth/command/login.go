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

// SkipTransaction keeps LoginCommand out of the bus-managed tx. The
// handler touches user.RecordFailedLogin / ResetFailedLogin (raw pool)
// — wrapping the whole thing in a write tx would self-deadlock under
// SQLite (raw-pool write waits for the tx this handler hasn't
// returned from). The handler's own writes are single-statement and
// safe to auto-commit individually.
func (LoginCommand) SkipTransaction() {}

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

// Brute-force lock thresholds. Hardcoded rather than env-driven because
// the right values are well-known and tuning them per-deployment hides
// the protection in config rather than in the code. Adjust here if a
// future deployment has a strong reason to differ.
const (
	loginLockThreshold = 5
	loginLockWindow    = 10 * time.Minute
	loginLockDuration  = 15 * time.Minute
)

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
	audit := shared.AuditCollectorFromContext(ctx)

	u, err := h.users.FindByNickname(ctx, cmd.Nickname)
	if err != nil {
		return LoginResult{}, err
	}

	// Always call Verify so an attacker timing the response can't tell
	// whether the nickname existed OR whether the account is locked:
	// every branch pays the bcrypt cost on a real hash. Decisions
	// below collapse all failure modes into one neutral AuthError.
	hash := h.dummyHash
	if u != nil {
		hash = u.PasswordHash
	}
	verifyErr := h.password.Verify(cmd.Password, hash)

	// Lock check happens AFTER Verify so timing stays uniform across
	// "locked", "wrong password", and "unknown user" — without this,
	// a locked-account response would skip Verify and become measurably
	// faster, leaking lock state.
	locked := u != nil && u.LockedUntil.Valid && time.Now().Before(u.LockedUntil.Time)

	if u == nil || verifyErr != nil {
		h.handleFailedLogin(ctx, audit, cmd.Nickname, u, locked)
		return LoginResult{}, &shared.AuthError{Message: "invalid credentials"}
	}

	if locked {
		// Correct password but account is in cooldown — still a
		// neutral error so the response shape doesn't reveal whether
		// the password was right. Audit records this separately so
		// operators can see "someone is hammering a locked account".
		audit.Record(shared.AuditEvent{
			Action:     "auth.login.blocked_while_locked",
			TargetType: "user",
			TargetID:   u.ID,
		})
		return LoginResult{}, &shared.AuthError{Message: "invalid credentials"}
	}

	// Successful login → clear the counter so the next failure cycle
	// starts fresh. Best-effort; a transient DB error here shouldn't
	// block an otherwise-valid login.
	_ = h.users.ResetFailedLogin(ctx, u.ID)

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

	// Record success only after the token is actually issued and saved.
	// audit.md defines auth.login.succeeded as "po vydání tokenu" — emitting
	// it before tokens.Save would log a success for a login that then failed
	// on the save and returned an error to the caller.
	audit.Record(shared.AuditEvent{
		Action:     "auth.login.succeeded",
		TargetType: "user",
		TargetID:   u.ID,
	})

	return LoginResult{
		User:             *u,
		AccessToken:      accessToken,
		AccessExpiresIn:  accessExpiresIn,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: expiresAt,
	}, nil
}

// handleFailedLogin records the auth.login.failed event, bumps the
// brute-force counter for known users (unless already locked), and emits
// auth.account.locked when this attempt is the one that crosses the
// threshold. Skips the counter bump during cooldown so repeated attempts
// against a locked account don't keep extending the lock.
func (h *LoginHandler) handleFailedLogin(
	ctx context.Context,
	audit *shared.AuditCollector,
	nickname string,
	u *user.User,
	locked bool,
) {
	failEvent := shared.AuditEvent{
		Action:   "auth.login.failed",
		Metadata: map[string]any{"nickname": nickname},
	}
	if u != nil {
		failEvent.TargetType = "user"
		failEvent.TargetID = u.ID
	}
	audit.Record(failEvent)

	if u == nil || locked {
		return
	}

	lockedAt, _ := h.users.RecordFailedLogin(
		ctx, u.ID, loginLockThreshold, loginLockWindow, loginLockDuration,
	)
	if lockedAt == nil {
		return
	}

	audit.Record(shared.AuditEvent{
		Action:     "auth.account.locked",
		TargetType: "user",
		TargetID:   u.ID,
		Metadata: map[string]any{
			"locked_until": lockedAt.UTC().Format(time.RFC3339),
		},
	})
}
