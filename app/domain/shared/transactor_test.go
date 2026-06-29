package shared

import (
	"context"
	"errors"
	"testing"
)

type fakeTransactor struct {
	began, committed, rolledBack int
	beganForbidden               bool
}

func (f *fakeTransactor) BeginTx(ctx context.Context) (context.Context, error) {
	f.began++
	if IsTxForbidden(ctx) {
		f.beganForbidden = true
	}
	return ctx, nil
}
func (f *fakeTransactor) Commit(context.Context) error   { f.committed++; return nil }
func (f *fakeTransactor) Rollback(context.Context) error { f.rolledBack++; return nil }

func TestWithTx_NoTransactor_ReturnsErr(t *testing.T) {
	err := WithTx(context.Background(), func(context.Context) error { return nil })
	if !errors.Is(err, ErrTxUnavailable) {
		t.Fatalf("want ErrTxUnavailable, got %v", err)
	}
}

func TestWithTx_Commit(t *testing.T) {
	tr := &fakeTransactor{}
	ctx := ContextWithTransactor(context.Background(), tr)
	if err := WithTx(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if tr.began != 1 || tr.committed != 1 || tr.rolledBack != 0 {
		t.Fatalf(
			"began=%d committed=%d rolledBack=%d, want 1/1/0",
			tr.began,
			tr.committed,
			tr.rolledBack,
		)
	}
}

func TestWithTx_FnError_RollsBack(t *testing.T) {
	tr := &fakeTransactor{}
	ctx := ContextWithTransactor(context.Background(), tr)
	boom := errors.New("boom")
	if err := WithTx(ctx, func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if tr.committed != 0 || tr.rolledBack != 1 {
		t.Fatalf("committed=%d rolledBack=%d, want 0/1", tr.committed, tr.rolledBack)
	}
}

func TestWithTx_Panic_RollsBack(t *testing.T) {
	tr := &fakeTransactor{}
	ctx := ContextWithTransactor(context.Background(), tr)
	defer func() {
		if recover() == nil {
			t.Fatal("a panic in fn must propagate out of WithTx")
		}
		if tr.committed != 0 || tr.rolledBack != 1 {
			t.Fatalf(
				"after panic: committed=%d rolledBack=%d, want 0/1",
				tr.committed,
				tr.rolledBack,
			)
		}
	}()
	_ = WithTx(ctx, func(context.Context) error { panic("kaboom") })
}

// WithTx opens the tx despite a surrounding ContextForbidTx zone (BeginTx runs on an
// allow-tx ctx), but RE-FORBIDS implicit tx for fn's scope: fn reaches the live tx via
// Conn(ctx), yet an accidental raw BeginTx inside fn still fails closed.
func TestWithTx_ReforbidsInsideFn(t *testing.T) {
	tr := &fakeTransactor{}
	ctx := ContextWithTransactor(ContextForbidTx(context.Background()), tr)
	err := WithTx(ctx, func(fnCtx context.Context) error {
		if !IsTxForbidden(fnCtx) {
			t.Fatal("inside WithTx the implicit-tx forbid must be re-applied for fn's scope")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if tr.beganForbidden {
		t.Fatal("BeginTx must run on an allow-tx ctx, not the forbidden one")
	}
}

// Nesting WithTx is refused — a second BEGIN IMMEDIATE on SQLite's single writer is a
// footgun; the inner call returns ErrNestedTx without opening a second tx.
func TestWithTx_NestingRefused(t *testing.T) {
	tr := &fakeTransactor{}
	ctx := ContextWithTransactor(context.Background(), tr)
	var innerErr error
	err := WithTx(ctx, func(fnCtx context.Context) error {
		innerErr = WithTx(fnCtx, func(context.Context) error { return nil })
		return nil
	})
	if err != nil {
		t.Fatalf("outer WithTx: %v", err)
	}
	if !errors.Is(innerErr, ErrNestedTx) {
		t.Fatalf("nested WithTx must return ErrNestedTx, got %v", innerErr)
	}
	if tr.began != 1 {
		t.Fatalf("nested WithTx must not open a second tx: began=%d want 1", tr.began)
	}
}
