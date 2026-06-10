package audit_test

import (
	"context"
	"testing"
	"time"

	"gokick/app/domain/shared"

	"github.com/google/uuid"
)

// TestRepository_SaveSurvivesBusinessRollback pins the core audit guarantee:
// Save writes through the raw connection pool (r.DB.DB()), NOT the tx on the
// context (r.Conn(ctx)). In production AuditMiddleware sits OUTSIDE
// TransactionMiddleware, so Save runs after the business tx has already rolled
// back while the ctx still carries the (now dead) tx. We reproduce that exactly:
//
//	BeginTx -> write control row via r.Conn(txCtx) -> Rollback -> Save(txCtx)
//
// The audit row must persist (raw pool ignores the dead tx) while the control
// row written through the tx must be gone. If Save ever regressed to
// r.Conn(ctx), step 4 would hit the rolled-back tx and return sql.ErrTxDone,
// failing this test.
//
// Closes: app-events-audit-22, app-events-audit-28, roadmap-76.
func TestRepository_SaveSurvivesBusinessRollback(t *testing.T) {
	ctx := context.Background()
	r, fx := newRepo(t)

	// Start a business transaction on the context.
	txCtx, err := fx.DB.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	// Control row: written through r.Conn(txCtx), so it JOINS the business tx
	// and must disappear on rollback. This is the discriminator — it proves the
	// rollback we trigger actually undoes tx-joined writes to this very table.
	controlID := uuid.New().String()
	if _, err := r.Conn(txCtx).ExecContext(txCtx,
		`INSERT INTO audit_log (id, action, created_at) VALUES (?, ?, ?)`,
		controlID, "control.tx_joined", time.Now()); err != nil {
		t.Fatalf("write control row in tx: %v", err)
	}

	// Roll the business work back. The tx is now closed, but txCtx still holds
	// the dead *sqlx.Tx — mirroring the production middleware ordering.
	if err := fx.DB.Rollback(txCtx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// Audit write happens on the SAME (dead-tx) context. Save must use the raw
	// pool and succeed regardless of the rolled-back tx.
	auditID := uuid.New().String()
	rec := &shared.AuditRecord{
		ID:        auditID,
		Action:    "auth.login.failed",
		CreatedAt: time.Now(),
	}
	if err := r.Save(txCtx, rec); err != nil {
		t.Fatalf("audit Save after rollback (regressed to r.Conn(ctx)?): %v", err)
	}

	// Read back via the raw pool: audit row present, control row gone.
	var auditCount int
	if err := fx.DB.DB().GetContext(ctx, &auditCount,
		`SELECT COUNT(*) FROM audit_log WHERE id=?`, auditID); err != nil {
		t.Fatalf("count audit row: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("audit row did not survive business rollback: got %d want 1", auditCount)
	}

	var controlCount int
	if err := fx.DB.DB().GetContext(ctx, &controlCount,
		`SELECT COUNT(*) FROM audit_log WHERE id=?`, controlID); err != nil {
		t.Fatalf("count control row: %v", err)
	}
	if controlCount != 0 {
		t.Fatalf(
			"control row survived rollback — rollback did not take effect: got %d want 0",
			controlCount,
		)
	}
}

// TestRepository_SaveIsAppendOnlyInsert pins that Save is a plain INSERT, never
// an upsert (INSERT OR REPLACE / ON CONFLICT DO UPDATE). Saving a second record
// that reuses an existing primary key must fail with a constraint error, and the
// original row must remain byte-for-byte unchanged. This is the "never modifies
// existing rows" regression guard: it fails the moment Save is switched to an
// upsert that would silently overwrite an existing audit entry.
//
// Closes: app-events-audit-15.
func TestRepository_SaveIsAppendOnlyInsert(t *testing.T) {
	ctx := context.Background()
	r, fx := newRepo(t)

	id := uuid.New().String()
	first := &shared.AuditRecord{
		ID:        id,
		Action:    "user.created",
		CreatedAt: time.Now(),
	}
	if err := r.Save(ctx, first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Second Save reuses the same id with a different action. A plain INSERT
	// rejects this on the PRIMARY KEY; an upsert would silently overwrite.
	second := &shared.AuditRecord{
		ID:        id,
		Action:    "user.deleted",
		CreatedAt: time.Now(),
	}
	if err := r.Save(ctx, second); err == nil {
		t.Fatalf(
			"second Save with duplicate id succeeded — Save is no longer append-only (upsert?)",
		)
	}

	// The stored row must still reflect the FIRST write, untouched.
	var gotAction string
	if err := fx.DB.DB().GetContext(ctx, &gotAction,
		`SELECT action FROM audit_log WHERE id=?`, id); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gotAction != "user.created" {
		t.Fatalf("existing audit row was modified: action=%q want %q", gotAction, "user.created")
	}

	// And there is exactly one row for that id (the duplicate was not appended).
	var count int
	if err := fx.DB.DB().GetContext(ctx, &count,
		`SELECT COUNT(*) FROM audit_log WHERE id=?`, id); err != nil {
		t.Fatalf("count rows for id: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count for id: got %d want 1", count)
	}
}
