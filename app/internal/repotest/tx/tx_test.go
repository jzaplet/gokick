package tx_test

import (
	"context"
	"strings"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// The no-transaction zone guard: BeginTx must fail closed when ctx is marked by
// shared.ContextForbidTx (a durable run handler ctx), so an accidental transaction
// in a long-running run surfaces immediately instead of holding locks for the
// run's lifetime (on SQLite the global write lock — every other write freezes).
func TestTransactor_BeginTx_FailsClosedInNoTxZone(t *testing.T) {
	fx := testfx.New(t)

	// A normal ctx → BeginTx succeeds (and we roll it back).
	if ctx, err := fx.Tx.BeginTx(context.Background()); err != nil {
		t.Fatalf("BeginTx on a normal ctx must succeed, got %v", err)
	} else if rbErr := fx.Tx.Rollback(ctx); rbErr != nil {
		t.Fatalf("rollback: %v", rbErr)
	}

	// A no-transaction zone → BeginTx fails closed, no tx opened.
	if _, err := fx.Tx.BeginTx(shared.ContextForbidTx(context.Background())); err == nil {
		t.Fatal("BeginTx in a no-transaction zone must fail closed")
	} else if !strings.Contains(err.Error(), "no-transaction zone") {
		t.Fatalf("error must name the no-transaction zone, got %v", err)
	}
}
