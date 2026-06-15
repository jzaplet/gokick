package command

import (
	"context"
	"errors"
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
// and expiry paths call tokens.DeleteByUserID and then return an AuthError.
// DeleteByUserID is tx-aware (r.Conn), so under a bus tx that AuthError would
// roll the deletion BACK and defeat the force-logout the theft response depends
// on. Running outside the tx lets the cleanup auto-commit and persist. The
// happy-path rotation (Save new → MarkUsed old) is consequently non-atomic,
// which is safe BECAUSE of that order: the new token is persisted before the CAS
// consumes the old one, so a failed MarkUsed after Save leaves only an unused
// orphan token (cleaned up by a later theft sweep or by expiry) rather than
// marking the old token used and force-logging-out a legitimate client on retry.
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
		// Record the security event BEFORE the revocation so the theft is audited
		// even if the delete fails. If the delete DOES fail, surface that error (→
		// 5xx, cookie kept) instead of the AuthError: returning "reuse detected"
		// while the tokens are still live would falsely tell the client it is
		// logged out, and a 5xx lets the next retry re-attempt the revocation.
		audit.Record(shared.AuditEvent{
			Action:     "auth.token.theft_detected",
			TargetType: "user",
			TargetID:   existing.UserID,
			Metadata:   map[string]any{"reason": "reused_after_rotation"},
		})
		if err := h.tokens.DeleteByUserID(ctx, existing.UserID); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, &shared.AuthError{Message: "refresh token reuse detected"}
	}

	if time.Now().After(existing.ExpiresAt) {
		// Best-effort cleanup (note the discarded error), unlike the theft branches
		// which surface a failed revocation: an expired token is already rejected by
		// the expiry check above on any retry, so a failed delete here is harmless.
		// Return the AuthError (→ 401, cookie cleared) regardless; the orphaned
		// expired row is swept by a later rotation or retention.
		_ = h.tokens.DeleteByUserID(ctx, existing.UserID)
		return LoginResult{}, &shared.AuthError{Message: "refresh token expired"}
	}

	u, err := h.users.FindByID(ctx, existing.UserID)
	if err != nil {
		// Only a genuine "user deleted" is a definitive auth failure that should
		// end the session: FindByID returns *ValidationError on not-found, and the
		// raw driver error for anything transient (SQLITE_BUSY, cancelled ctx). A
		// transient error must propagate raw → 5xx → the refresh cookie is kept,
		// or a momentary blip during this lookup would clear the cookie + hint and
		// durably log out a still-valid session — the very regression this fix
		// prevents on every OTHER branch of this handler.
		var notFound *shared.ValidationError
		if errors.As(err, &notFound) {
			return LoginResult{}, &shared.AuthError{Message: "user no longer exists"}
		}

		return LoginResult{}, err
	}

	// Mint the replacement token pair and persist the new refresh token FIRST,
	// before consuming the old one. Order matters: a transient Save failure here
	// leaves the old token untouched, so the next attempt rotates cleanly —
	// whereas marking the old token used first and then failing Save would
	// force-logout a legitimate client on retry (the old cookie would now read as
	// reused). The new token is not handed to any client until this handler
	// returns success, so nothing can present it during the Save→MarkUsed window.
	accessToken, accessExpiresIn, err := h.jwt.GenerateAccessToken(&shared.AuthClaims{
		UserID:   u.ID,
		Role:     u.Role,
		Nickname: u.Nickname,
		Email:    u.Email,
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

	// Now consume the old token: atomically mark it used. If the update touched 0
	// rows, a concurrent request rotated it first — treat that as theft (the raw
	// token is in two places) and revoke everything, including the token just
	// saved above (DeleteByUserID drops all of the user's tokens). Audit before
	// the delete and surface a delete failure, for the same reasons as the
	// reused-token branch above.
	marked, err := h.tokens.MarkUsed(ctx, hash)
	if err != nil {
		return LoginResult{}, err
	}
	if !marked {
		// A concurrent rotation of the SAME cookie lost the CAS race. Because Save
		// ran before MarkUsed, the race-winner's replacement token is already
		// persisted and so is wiped here too (DeleteByUserID drops ALL of the
		// user's tokens) — making theft revocation deterministic instead of
		// race-dependent. The old MarkUsed→Save order could let the winner's Save
		// land AFTER this delete, leaving a possibly-leaked refresh token live;
		// the new order bounds a race-winning attacker to a single access-token
		// lifetime instead of potentially-indefinite refresh retention. The cost is
		// UX: two legitimate tabs refreshing at once are both logged out (their
		// per-context client single-flight cannot coordinate across tabs).
		// Removing that false positive needs cross-tab refresh coordination — a
		// roadmap item, not a change to this CAS.
		audit.Record(shared.AuditEvent{
			Action:     "auth.token.theft_detected",
			TargetType: "user",
			TargetID:   existing.UserID,
			Metadata:   map[string]any{"reason": "concurrent_rotation_race"},
		})
		if err := h.tokens.DeleteByUserID(ctx, existing.UserID); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, &shared.AuthError{Message: "refresh token reuse detected"}
	}

	return LoginResult{
		User:             *u,
		AccessToken:      accessToken,
		AccessExpiresIn:  accessExpiresIn,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: expiresAt,
	}, nil
}
