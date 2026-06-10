package middleware

import (
	"context"
	"errors"
	"testing"
)

// stubTx records BeginTx / Commit / Rollback calls so we can assert
// SkipsTransaction actually bypasses the middleware. commitErr lets a test
// simulate a failing Commit (used by the DispatchEvents+Transaction integration
// test to prove events are discarded on commit failure).
type stubTx struct {
	beginCalls    int
	commitCalls   int
	rollbackCalls int
	beginErr      error
	commitErr     error
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
	return s.commitErr
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

// NOTE: the guarantee that the real LoginCommand / RefreshTokenCommand stay
// opted out of the tx is enforced by the behavioral test
// TestLoginHandler_DoesNotDeadlockUnderCommandBus (auth/command) — it dispatches
// the real command through the real bus and deadlocks if SkipTransaction() is
// removed. A compile-time assertion can't live here: arch-lint forbids the
// bus_middleware package from importing application/auth/command, so any
// assertion in this file could only reference a local dummy and would prove
// nothing about the production commands.

// Sanity: a SkipsTransaction-implementing command still composes cleanly with
// the middleware and the handler result is returned untouched.
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
}
