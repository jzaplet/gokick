package command

import (
	"context"
	"fmt"

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
// happy-path rotation is consequently non-atomic, which is safe because of the
// issue-before-consume ordering token.Rotator enforces (see its doc comment).
func (RefreshTokenCommand) SkipTransaction() {}

type RefreshTokenHandler struct {
	users   user.Repository
	tokens  token.Repository
	jwt     shared.TokenService
	rotator *token.Rotator
}

func NewRefreshTokenHandler(
	users user.Repository,
	tokens token.Repository,
	jwt shared.TokenService,
) *RefreshTokenHandler {
	return &RefreshTokenHandler{
		users:   users,
		tokens:  tokens,
		jwt:     jwt,
		rotator: token.NewRotator(tokens),
	}
}

// Handle orchestrates one refresh: token.Rotator runs the rotation +
// theft-detection state machine (the token-side invariants live there); this
// handler supplies the replacement-session step and maps the typed outcome to
// the auth error model — including the theft response, whose audit/revoke
// ordering is an application concern.
func (h *RefreshTokenHandler) Handle(
	ctx context.Context,
	cmd RefreshTokenCommand,
) (IssuedSession, error) {
	var res IssuedSession
	outcome, err := h.rotator.Rotate(
		ctx,
		h.jwt.HashRefreshToken(cmd.RawToken),
		func(ctx context.Context, userID string) error {
			u, err := h.users.FindByID(ctx, userID)
			if err != nil {
				// A transient failure (SQLITE_BUSY, cancelled ctx) must propagate raw →
				// 5xx → the refresh cookie is KEPT. A momentary blip during this lookup
				// must never end the session, or it durably logs out a still-valid
				// client. Only a genuine "user is gone" (u == nil below) is a
				// definitive auth failure.
				return err
			}
			if u == nil {
				// FindByID's not-found contract is (nil, nil): the user's row is
				// genuinely gone. That IS a definitive auth failure — end the session.
				return &shared.AuthError{Message: "user no longer exists"}
			}
			if !u.Active {
				// Deactivated after this session was issued: end it. Access tokens
				// are stateless so deactivation bites at the next refresh (within
				// one access-token TTL), not instantly — immediate revocation would
				// be a separate, heavier change.
				return &shared.AuthError{Message: "account is inactive"}
			}
			// issueSession's Save is its final write — the contract Rotate's
			// issue-before-consume ordering relies on.
			res, err = issueSession(ctx, h.jwt, h.tokens, u)
			return err
		},
	)
	if err != nil {
		return IssuedSession{}, err
	}

	switch outcome.Outcome {
	case token.RotationRotated:
		return res, nil
	case token.RotationInvalid:
		return IssuedSession{}, &shared.AuthError{Message: "invalid refresh token"}
	case token.RotationExpired:
		return IssuedSession{}, &shared.AuthError{Message: "refresh token expired"}
	case token.RotationTheft:
		return IssuedSession{}, h.revokeAllAsTheft(ctx, outcome.UserID, outcome.TheftReason)
	default:
		return IssuedSession{}, fmt.Errorf("unexpected rotation outcome %q", outcome.Outcome)
	}
}

// revokeAllAsTheft audits a token-theft event and force-logs-out the user on
// all devices, returning the AuthError to surface. Both theft outcomes — reuse
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
