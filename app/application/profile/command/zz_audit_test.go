package command

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// TestChangePasswordHandler_RecordsAuditEvent closes claim app-events-audit-44:
// a successful password change records exactly one audit event with action
// "user.password_changed", target type "user", the user's ID, and no metadata.
func TestChangePasswordHandler_RecordsAuditEvent(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pwd_audit.db"))
	u := fx.SeedUser(t, "alice", "old-password", "user")

	// Both injections are required: claims so the handler reaches the success
	// path, and a real collector so AuditCollectorFromContext does not return a
	// throwaway (which would silently swallow the recorded event).
	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	})
	collector := &shared.AuditCollector{}
	auditCtx := shared.ContextWithAuditCollector(authCtx, collector)

	handler := NewChangePasswordHandler(fx.Users, fx.Hasher)
	err := handler.Handle(auditCtx, ChangePasswordCommand{
		OldPassword: "old-password",
		NewPassword: "brand-new-password",
	})
	// Record only fires after Update succeeds; assert the success path was
	// actually reached so an early-return failure cannot masquerade as "no event".
	if err != nil {
		t.Fatalf("change password: %v", err)
	}

	drained := collector.Drain()
	if len(drained) != 1 {
		t.Fatalf("expected exactly 1 audit event, got %d: %+v", len(drained), drained)
	}

	e := drained[0]
	if e.Action != "user.password_changed" {
		t.Errorf("expected action user.password_changed, got %q", e.Action)
	}
	if e.TargetType != "user" {
		t.Errorf("expected target type user, got %q", e.TargetType)
	}
	if e.TargetID != u.ID {
		t.Errorf("expected target id %q, got %q", u.ID, e.TargetID)
	}
	// "no metadata" — len covers both nil and an empty-but-non-nil map.
	if len(e.Metadata) != 0 {
		t.Errorf("expected no metadata, got %+v", e.Metadata)
	}
}
