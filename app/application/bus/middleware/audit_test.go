package middleware

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"gokick/app/domain/shared"
)

type captureAudit struct {
	mu      sync.Mutex
	records []shared.AuditRecord
	failOn  string // if set, Save returns error for actions starting with this prefix
}

func (c *captureAudit) Save(_ context.Context, rec *shared.AuditRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failOn != "" && len(rec.Action) >= len(c.failOn) &&
		rec.Action[:len(c.failOn)] == c.failOn {
		return errors.New("audit boom")
	}
	c.records = append(c.records, *rec)
	return nil
}

func (c *captureAudit) drained() []shared.AuditRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]shared.AuditRecord(nil), c.records...)
}

func silent() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The mid must drain & persist events even when the handler returned an
// error — that's the whole point of recording login_failed.
func TestAuditMiddleware_FlushesEvenOnHandlerError(t *testing.T) {
	t.Parallel()
	audit := &captureAudit{}
	mw := AuditMiddleware(silent(), audit)

	handler := func(ctx context.Context) (any, error) {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: "auth.login.failed"})
		return nil, errors.New("invalid credentials")
	}

	_, err := mw(t.Context(), "Login", nil, handler)
	if err == nil || err.Error() != "invalid credentials" {
		t.Fatalf("handler error must propagate, got %v", err)
	}
	recs := audit.drained()
	if len(recs) != 1 || recs[0].Action != "auth.login.failed" {
		t.Fatalf("expected auth.login.failed persisted on error, got %+v", recs)
	}
}

func TestAuditMiddleware_StampsActorAndIPFromContext(t *testing.T) {
	t.Parallel()
	audit := &captureAudit{}
	mw := AuditMiddleware(silent(), audit)

	ctx := shared.ContextWithClaims(t.Context(), &shared.AuthClaims{UserID: "user-7"})
	ctx = shared.ContextWithActorIP(ctx, "203.0.113.5")

	handler := func(ctx context.Context) (any, error) {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
			Action:     "user.created",
			TargetType: "user",
			TargetID:   "new-id",
			Metadata:   map[string]any{"role": "admin"},
		})
		return nil, nil
	}

	if _, err := mw(ctx, "CreateUser", nil, handler); err != nil {
		t.Fatalf("handler: %v", err)
	}

	recs := audit.drained()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	rec := recs[0]
	if rec.ActorUserID == nil || *rec.ActorUserID != "user-7" {
		t.Fatalf("actor: %v", rec.ActorUserID)
	}
	if rec.ActorIP == nil || *rec.ActorIP != "203.0.113.5" {
		t.Fatalf("actor_ip: %v", rec.ActorIP)
	}
	if rec.TargetID == nil || *rec.TargetID != "new-id" {
		t.Fatalf("target_id: %v", rec.TargetID)
	}
	if len(rec.Metadata) == 0 {
		t.Fatal("metadata must be JSON-encoded, got empty")
	}
}

// Audit write failures must NOT turn a successful business operation
// into a 500 — the trail is best-effort by design.
func TestAuditMiddleware_PersistFailureDoesNotShadowHandlerResult(t *testing.T) {
	t.Parallel()
	audit := &captureAudit{failOn: "user."}
	mw := AuditMiddleware(silent(), audit)

	handler := func(ctx context.Context) (any, error) {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: "user.created"})
		return "ok", nil
	}

	got, err := mw(t.Context(), "CreateUser", nil, handler)
	if err != nil {
		t.Fatalf("audit failure must not propagate, got %v", err)
	}
	if got != "ok" {
		t.Fatalf("handler result lost: %v", got)
	}
}

func TestAuditMiddleware_NoRecordsIsNoOp(t *testing.T) {
	t.Parallel()
	audit := &captureAudit{}
	mw := AuditMiddleware(silent(), audit)
	handler := func(_ context.Context) (any, error) { return nil, nil }

	if _, err := mw(t.Context(), "Cmd", nil, handler); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if recs := audit.drained(); len(recs) != 0 {
		t.Fatalf("no records expected, got %d", len(recs))
	}
}
