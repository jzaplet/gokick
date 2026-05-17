package di

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"gokick/app/domain/token"
)

// stubTokens is a no-op TokenRepository for invoking the production job
// collector without a real DB. We only care the registered slice validates.
type stubTokens struct{}

func (stubTokens) Save(context.Context, *token.RefreshToken) error { return nil }
func (stubTokens) FindByHash(context.Context, string) (*token.RefreshToken, error) {
	return nil, nil
}
func (stubTokens) MarkUsed(context.Context, string) (bool, error) { return true, nil }
func (stubTokens) DeleteByUserID(context.Context, string) error   { return nil }
func (stubTokens) DeleteExpired(context.Context) error            { return nil }

// Catches a "someone added a duplicate name (or invalid interval, or nil Fn)
// to provideSchedulerJobs" regression at test time instead of at process
// startup — the constructor's validation error would otherwise only surface
// when Wire bubbles it up from CreateApplication.
func TestProvideScheduler_AcceptsRegisteredJobs(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jobs := provideSchedulerJobs(stubTokens{})
	if _, err := provideScheduler(logger, jobs); err != nil {
		t.Fatalf("provideScheduler rejected its registered jobs: %v", err)
	}
}
