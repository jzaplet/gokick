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

// WithTx is the blessed escape from a ContextForbidTx zone: it must open the tx on
// an allow-tx ctx (so the run worker's forbid marker doesn't block a deliberate
// short tx), while a raw BeginTx on the surrounding ctx still fails closed.
func TestWithTx_OverridesForbidMarker(t *testing.T) {
	tr := &fakeTransactor{}
	ctx := ContextWithTransactor(ContextForbidTx(context.Background()), tr)
	err := WithTx(ctx, func(txCtx context.Context) error {
		if IsTxForbidden(txCtx) {
			t.Fatal("inside WithTx the forbid marker must be cleared")
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
