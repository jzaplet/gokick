package token

import (
	"context"
	"time"
)

// RotationOutcome is what the rotation state machine decided about the
// presented token. String-typed so a logged/audited outcome reads as itself.
type RotationOutcome string

const (
	// RotationRotated — the token was valid and has been consumed; the
	// replacement issued by the callback is now the live token.
	RotationRotated RotationOutcome = "rotated"
	// RotationInvalid — the hash matches no stored token.
	RotationInvalid RotationOutcome = "invalid"
	// RotationExpired — the token exists but its lifetime is over.
	RotationExpired RotationOutcome = "expired"
	// RotationTheft — the token was presented from two places; see TheftReason.
	RotationTheft RotationOutcome = "theft"
)

// Theft reasons, recorded in the audit trail's metadata by the caller.
const (
	// TheftReasonReuse — a token that was already rotated is being presented
	// again: the raw value exists in two places, assume compromise.
	TheftReasonReuse = "reused_after_rotation"
	// TheftReasonConcurrentRace — two requests rotated the same token at once
	// and this one lost the MarkUsed CAS. Same two-places conclusion.
	TheftReasonConcurrentRace = "concurrent_rotation_race"
)

// RotationResult is the typed outcome of one Rotate call. UserID is set for
// every outcome that identified a token owner (everything except invalid), so
// the caller can respond — revoke on theft — without re-reading the token.
type RotationResult struct {
	Outcome     RotationOutcome
	UserID      string
	TheftReason string // set only when Outcome == RotationTheft
}

// Rotator is the refresh-token rotation + theft-detection state machine. It
// owns the token-side invariants — reuse detection, expiry, and the
// issue-before-consume ordering — while the caller supplies the one step the
// token context cannot know (minting the replacement session for the user)
// and maps the typed outcome to its own error/response model.
type Rotator struct {
	tokens Repository
}

func NewRotator(tokens Repository) *Rotator {
	return &Rotator{tokens: tokens}
}

// Rotate runs the state machine for the presented token hash.
//
// issueReplacement is invoked only for a valid, unconsumed token, and — the
// load-bearing ordering — BEFORE the old token is marked used. Its final write
// must be persisting the replacement token: a transient failure there then
// leaves the old token untouched, so the next attempt rotates cleanly. The
// reverse order (consume first, issue second) would, on a failed issue,
// force-logout a legitimate client on retry — its cookie would read as reused.
// The replacement is not handed to any client until the caller returns
// success, so nothing can present it during the issue→MarkUsed window. An
// error from the callback propagates raw (no outcome), leaving the old token
// unconsumed.
//
// After the callback, the old token is consumed via the MarkUsed CAS. A missed
// CAS means a concurrent request rotated the same raw token first — theft.
// Because issue ran before MarkUsed, the race-winner's replacement is already
// persisted, so the caller's revocation (delete all of the user's tokens)
// deterministically wipes it too, bounding a race-winning attacker to a single
// access-token lifetime. The cost is UX: two legitimate tabs refreshing at
// once are both logged out (per-tab single-flight cannot coordinate across
// tabs); removing that false positive needs cross-tab refresh coordination — a
// roadmap item, not a change to this CAS.
//
// The theft response itself (audit + revoke-all) stays with the caller: it is
// an application concern with its own ordering rules, and this seam only names
// the verdict.
func (r *Rotator) Rotate(
	ctx context.Context,
	hash string,
	issueReplacement func(ctx context.Context, userID string) error,
) (RotationResult, error) {
	existing, err := r.tokens.FindByHash(ctx, hash)
	if err != nil {
		return RotationResult{}, err
	}
	if existing == nil {
		return RotationResult{Outcome: RotationInvalid}, nil
	}

	// Theft detection: a token that was already rotated is being presented again.
	if existing.UsedAt != nil {
		return RotationResult{
			Outcome:     RotationTheft,
			UserID:      existing.UserID,
			TheftReason: TheftReasonReuse,
		}, nil
	}

	if time.Now().After(existing.ExpiresAt) {
		// Best-effort cleanup (note the discarded error): an expired token is
		// already rejected by the expiry check above on any retry, so a failed
		// delete here is harmless — the orphaned expired row is swept by a
		// later rotation or retention. The expired outcome stands regardless.
		_ = r.tokens.DeleteByUserID(ctx, existing.UserID)
		return RotationResult{Outcome: RotationExpired, UserID: existing.UserID}, nil
	}

	if err := issueReplacement(ctx, existing.UserID); err != nil {
		return RotationResult{}, err
	}

	marked, err := r.tokens.MarkUsed(ctx, hash)
	if err != nil {
		return RotationResult{}, err
	}
	if !marked {
		return RotationResult{
			Outcome:     RotationTheft,
			UserID:      existing.UserID,
			TheftReason: TheftReasonConcurrentRace,
		}, nil
	}

	return RotationResult{Outcome: RotationRotated, UserID: existing.UserID}, nil
}
