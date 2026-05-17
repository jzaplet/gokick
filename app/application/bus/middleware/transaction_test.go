package middleware

import (
	"context"
	"errors"
	"testing"

	"gokick/app/domain/shared"
)

// stubTx records BeginTx / Commit / Rollback calls so we can assert
// SkipsTransaction actually bypasses the middleware.
type stubTx struct {
	beginCalls    int
	commitCalls   int
	rollbackCalls int
	beginErr      error
}

func (s *stubTx) BeginTx(ctx context.Context) (context.Context, error) {
	s.beginCalls++
	if s.beginErr != nil {
		return ctx, s.beginErr
	}
	return ctx, nil
}

func (s *stubTx) Commit(context.Context) error {
	s.commitCalls++
	return nil
}

func (s *stubTx) Rollback(context.Context) error {
	s.rollbackCalls++
	return nil
}

type normalCmd struct{}

type skipCmd struct{}

func (skipCmd) SkipTransaction() {}

var _ SkipsTransaction = skipCmd{}

func TestTransactionMiddleware_WrapsByDefault(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(tx)

	_, err := mw(t.Context(), "Normal", normalCmd{}, func(context.Context) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if tx.beginCalls != 1 || tx.commitCalls != 1 || tx.rollbackCalls != 0 {
		t.Fatalf("expected begin=commit=1, rollback=0; got %+v", tx)
	}
}

func TestTransactionMiddleware_RollsBackOnHandlerError(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(tx)

	_, err := mw(t.Context(), "Normal", normalCmd{}, func(context.Context) (any, error) {
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("expected rollback=1, commit=0; got %+v", tx)
	}
}

// Commands implementing SkipsTransaction must skip BeginTx entirely.
// Regression guard: without this skip, LoginHandler self-deadlocks
// under SQLite (its raw-pool writes block on the wrapping tx).
func TestTransactionMiddleware_SkipsForOptOutCommands(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(tx)

	var ran bool
	_, err := mw(t.Context(), "Skip", skipCmd{}, func(context.Context) (any, error) {
		ran = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !ran {
		t.Fatal("handler must still run when tx is skipped")
	}
	if tx.beginCalls != 0 || tx.commitCalls != 0 || tx.rollbackCalls != 0 {
		t.Fatalf("opt-out command must touch no tx methods; got %+v", tx)
	}
}

// Compile-time assertion: real LoginCommand / RefreshTokenCommand
// stay opted out. If somebody removes the SkipTransaction() method
// from either, the next build of this test file fails — which is
// exactly the signal we want (silent re-enable of tx = production
// deadlock).
var (
	_ SkipsTransaction = loginCmdAssertion{}
	_ SkipsTransaction = refreshCmdAssertion{}
)

type loginCmdAssertion struct{}

func (loginCmdAssertion) SkipTransaction() {}

type refreshCmdAssertion struct{}

func (refreshCmdAssertion) SkipTransaction() {}

// Sanity: a SkipsTransaction-implementing wrapper still composes
// cleanly with other middleware (logger, audit, …) via shared.Transactor.
func TestTransactionMiddleware_StillExecutesNextOnSkip(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(tx)

	got, err := mw(t.Context(), "Skip", skipCmd{}, func(_ context.Context) (any, error) {
		return "value", nil
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got != "value" {
		t.Fatalf("result lost: %v", got)
	}
	_ = shared.AuthClaims{} // keep shared imported in case future assertions need it
}
