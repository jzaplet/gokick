package di

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"gokick/app/domain/token"
)

// stubTokens is a no-op TokenRepository used solely to instantiate the
// scheduler from production wiring; we only care whether provideScheduler
// accepts the registered Job slice.
type stubTokens struct{}

func (stubTokens) Save(context.Context, *token.RefreshToken) error  { return nil }
func (stubTokens) FindByHash(context.Context, string) (*token.RefreshToken, error) {
	return nil, nil
}
func (stubTokens) MarkUsed(context.Context, string) error      { return nil }
func (stubTokens) DeleteByUserID(context.Context, string) error { return nil }
func (stubTokens) DeleteExpired(context.Context) error          { return nil }

// Catches a "someone added a duplicate job name (or invalid interval, or nil
// Fn) to provideScheduler" regression at test time instead of at process
// startup. provideScheduler returns an error from its constructor — Wire just
// bubbles it up; this test exercises the production registry directly.
func TestProvideScheduler_AcceptsRegisteredJobs(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := provideScheduler(logger, stubTokens{}); err != nil {
		t.Fatalf("provideScheduler rejected its registered jobs: %v", err)
	}
}
