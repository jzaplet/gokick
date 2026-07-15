package token

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo is a recording in-memory token.Repository. calls captures the
// method-call order so tests can pin the issue-before-consume invariant.
type fakeRepo struct {
	byHash     map[string]*RefreshToken
	markUsedOK bool
	deleteErr  error
	calls      []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byHash: map[string]*RefreshToken{}, markUsedOK: true}
}

func (f *fakeRepo) Save(_ context.Context, t *RefreshToken) error {
	f.calls = append(f.calls, "Save")
	f.byHash[t.TokenHash] = t
	return nil
}

func (f *fakeRepo) FindByHash(_ context.Context, hash string) (*RefreshToken, error) {
	f.calls = append(f.calls, "FindByHash")
	return f.byHash[hash], nil
}

func (f *fakeRepo) MarkUsed(_ context.Context, _ string) (bool, error) {
	f.calls = append(f.calls, "MarkUsed")
	return f.markUsedOK, nil
}

func (f *fakeRepo) DeleteByUserID(_ context.Context, _ string) error {
	f.calls = append(f.calls, "DeleteByUserID")
	return f.deleteErr
}

func (f *fakeRepo) DeleteExpired(_ context.Context) error {
	f.calls = append(f.calls, "DeleteExpired")
	return nil
}

func seedToken(f *fakeRepo, hash string, expiresAt time.Time, used bool) *RefreshToken {
	t := NewRefreshToken("user-1", hash, expiresAt)
	if used {
		now := time.Now()
		t.UsedAt = &now
	}
	f.byHash[hash] = t
	return t
}

// noIssue fails the test if the rotation ever reaches the issue step.
func noIssue(t *testing.T) func(context.Context, string) error {
	t.Helper()
	return func(context.Context, string) error {
		t.Fatal("issueReplacement must not be called on this path")
		return nil
	}
}

func TestRotator_UnknownHashIsInvalid(t *testing.T) {
	repo := newFakeRepo()
	res, err := NewRotator(repo).Rotate(context.Background(), "nope", noIssue(t))
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if res.Outcome != RotationInvalid {
		t.Fatalf("outcome: got %q want %q", res.Outcome, RotationInvalid)
	}
}

func TestRotator_UsedTokenIsTheftReuse(t *testing.T) {
	repo := newFakeRepo()
	seedToken(repo, "h", time.Now().Add(time.Hour), true)

	res, err := NewRotator(repo).Rotate(context.Background(), "h", noIssue(t))
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if res.Outcome != RotationTheft || res.TheftReason != TheftReasonReuse {
		t.Fatalf(
			"got outcome %q reason %q, want theft/%s",
			res.Outcome,
			res.TheftReason,
			TheftReasonReuse,
		)
	}
	if res.UserID != "user-1" {
		t.Fatalf("theft outcome must carry the owner, got %q", res.UserID)
	}
}

// Expired: the outcome stands and the cleanup is best-effort — a failing
// DeleteByUserID must not turn the verdict into an error (the expiry check
// rejects any retry regardless).
func TestRotator_ExpiredCleansUpBestEffort(t *testing.T) {
	repo := newFakeRepo()
	seedToken(repo, "h", time.Now().Add(-time.Hour), false)
	repo.deleteErr = errors.New("database is locked")

	res, err := NewRotator(repo).Rotate(context.Background(), "h", noIssue(t))
	if err != nil {
		t.Fatalf("expired must not surface the best-effort delete error, got %v", err)
	}
	if res.Outcome != RotationExpired || res.UserID != "user-1" {
		t.Fatalf("got outcome %q user %q, want expired/user-1", res.Outcome, res.UserID)
	}
	if repo.calls[len(repo.calls)-1] != "DeleteByUserID" {
		t.Fatalf("expected a cleanup attempt, calls: %v", repo.calls)
	}
}

// The load-bearing ordering: the replacement is issued BEFORE the old token is
// consumed, so a transient issue failure leaves the old token rotatable.
func TestRotator_IssueRunsBeforeConsumeAndItsErrorPropagatesRaw(t *testing.T) {
	repo := newFakeRepo()
	seedToken(repo, "h", time.Now().Add(time.Hour), false)

	blip := errors.New("database is locked")
	_, err := NewRotator(repo).Rotate(context.Background(), "h",
		func(context.Context, string) error { return blip })
	if !errors.Is(err, blip) {
		t.Fatalf("expected the raw issue error to propagate, got %v", err)
	}
	for _, c := range repo.calls {
		if c == "MarkUsed" {
			t.Fatalf("old token must NOT be consumed when issue fails, calls: %v", repo.calls)
		}
	}
}

func TestRotator_HappyPathRotatesInOrder(t *testing.T) {
	repo := newFakeRepo()
	seedToken(repo, "h", time.Now().Add(time.Hour), false)

	var issuedFor string
	res, err := NewRotator(repo).Rotate(context.Background(), "h",
		func(_ context.Context, userID string) error {
			issuedFor = userID
			repo.calls = append(repo.calls, "issue")
			return nil
		})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if res.Outcome != RotationRotated || res.UserID != "user-1" {
		t.Fatalf("got outcome %q user %q, want rotated/user-1", res.Outcome, res.UserID)
	}
	if issuedFor != "user-1" {
		t.Fatalf("issue callback must receive the owner, got %q", issuedFor)
	}
	want := []string{"FindByHash", "issue", "MarkUsed"}
	if len(repo.calls) != len(want) {
		t.Fatalf("calls: got %v want %v", repo.calls, want)
	}
	for i, c := range want {
		if repo.calls[i] != c {
			t.Fatalf("call order: got %v want %v", repo.calls, want)
		}
	}
}

func TestRotator_LostCASIsTheftRace(t *testing.T) {
	repo := newFakeRepo()
	seedToken(repo, "h", time.Now().Add(time.Hour), false)
	repo.markUsedOK = false // a concurrent request consumed the token first

	res, err := NewRotator(repo).Rotate(context.Background(), "h",
		func(context.Context, string) error { return nil })
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if res.Outcome != RotationTheft || res.TheftReason != TheftReasonConcurrentRace {
		t.Fatalf(
			"got outcome %q reason %q, want theft/%s",
			res.Outcome, res.TheftReason, TheftReasonConcurrentRace,
		)
	}
}
