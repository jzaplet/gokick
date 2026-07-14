package command

import (
	"context"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
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
	tokens token.Repository
	jwt    shared.TokenService
}

func NewRefreshTokenHandler(
	users user.Repository,
	tokens token.Repository,
	jwt shared.TokenService,
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
) (IssuedSession, error) {
	hash := h.jwt.HashRefreshToken(cmd.RawToken)

	existing, err := h.tokens.FindByHash(ctx, hash)
	if err != nil {
		return IssuedSession{}, err
	}
	if existing == nil {
		return IssuedSession{}, &shared.AuthError{Message: "invalid refresh token"}
	}

	// Theft detection: a token that was already rotated is being presented again.
	// Assume credentials are compromised and log the user out on all devices.
	if existing.UsedAt != nil {
		return IssuedSession{}, h.revokeAllAsTheft(ctx, existing.UserID, "reused_after_rotation")
	}

	if time.Now().After(existing.ExpiresAt) {
		// Best-effort cleanup (note the discarded error), unlike the theft branches
		// which surface a failed revocation: an expired token is already rejected by
		// the expiry check above on any retry, so a failed delete here is harmless.
		// Return the AuthError (→ 401, cookie cleared) regardless; the orphaned
		// expired row is swept by a later rotation or retention.
		_ = h.tokens.DeleteByUserID(ctx, existing.UserID)
		return IssuedSession{}, &shared.AuthError{Message: "refresh token expired"}
	}

	u, err := h.users.FindByID(ctx, existing.UserID)
	if err != nil {
		// A transient failure (SQLITE_BUSY, cancelled ctx) must propagate raw →
		// 5xx → the refresh cookie is KEPT. A momentary blip during this lookup
		// must never end the session, or it durably logs out a still-valid client
		// — the very regression this handler guards on every other branch. Only a
		// genuine "user is gone" (u == nil below) is a definitive auth failure.
		return IssuedSession{}, err
	}
	if u == nil {
		// FindByID's not-found contract is (nil, nil): the user's row is genuinely
		// gone. That IS a definitive auth failure — end the session.
		return IssuedSession{}, &shared.AuthError{Message: "user no longer exists"}
	}

	// Mint the replacement token pair and persist the new refresh token FIRST,
	// before consuming the old one. Order matters: a transient Save failure here
	// leaves the old token untouched, so the next attempt rotates cleanly —
	// whereas marking the old token used first and then failing Save would
	// force-logout a legitimate client on retry (the old cookie would now read as
	// reused). The new token is not handed to any client until this handler
	// returns success, so nothing can present it during the Save→MarkUsed window.
	// issueSession's Save is its final write, so calling MarkUsed only after it
	// returns keeps that Save-before-MarkUsed order intact.
	res, err := issueSession(ctx, h.jwt, h.tokens, u)
	if err != nil {
		return IssuedSession{}, err
	}

	// Now consume the old token: atomically mark it used. If the update touched 0
	// rows, a concurrent request rotated it first — treat that as theft (the raw
	// token is in two places) and revoke everything, including the token just
	// saved above (DeleteByUserID drops all of the user's tokens). Audit before
	// the delete and surface a delete failure, for the same reasons as the
	// reused-token branch above.
	marked, err := h.tokens.MarkUsed(ctx, hash)
	if err != nil {
		return IssuedSession{}, err
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
		return IssuedSession{}, h.revokeAllAsTheft(ctx, existing.UserID, "concurrent_rotation_race")
	}

	return res, nil
}

// revokeAllAsTheft audits a token-theft event and force-logs-out the user on
// all devices, returning the AuthError to surface. Both theft branches — reuse
// after rotation, and the concurrent-rotation CAS race — share this exact
// response, differing only by reason. The event is recorded BEFORE the revoke
// so the theft is audited even if the delete fails; a delete failure is
// surfaced (→ 5xx, cookie kept) instead of the AuthError, because telling the
// client it is logged out while its tokens are still live is worse than a
// retryable 5xx that lets the next attempt re-run the revocation.
func (h *RefreshTokenHandler) revokeAllAsTheft(ctx context.Context, userID, reason string) error {
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "auth.token.theft_detected",
		TargetType: "user",
		TargetID:   userID,
		Metadata:   map[string]any{"reason": reason},
	})
	if err := h.tokens.DeleteByUserID(ctx, userID); err != nil {
		return err
	}
	return &shared.AuthError{Message: "refresh token reuse detected"}
}
