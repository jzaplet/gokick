package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// auditEventsByAction returns the subset of drained events matching action.
func auditEventsByAction(events []shared.AuditEvent, action string) []shared.AuditEvent {
	var out []shared.AuditEvent
	for _, e := range events {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// app-events-audit-41: CreateUserHandler emits audit action user.created with
// metadata {role}.
func TestCreateUserHandler_RecordsUserCreatedAudit(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit_user_created.db"))

	collector := &shared.AuditCollector{}
	// EventCollector is also needed: Handle calls EventCollectorFromContext too,
	// but it returns a throwaway when absent, so only the audit collector is wired.
	ctx := shared.ContextWithAuditCollector(context.Background(), collector)

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	if err := h.Handle(ctx, CreateUserCommand{
		Nickname: "bob",
		Password: "secret12",
		Email:    "bob@example.com",
		Role:     "admin",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	saved, err := fx.Users.FindByNickname(ctx, "bob")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if saved == nil {
		t.Fatal("expected user persisted")
	}

	created := auditEventsByAction(collector.Drain(), "user.created")
	if len(created) != 1 {
		t.Fatalf("user.created records: got %d want 1", len(created))
	}
	rec := created[0]
	if rec.TargetType != "user" {
		t.Fatalf("target_type: got %q want user", rec.TargetType)
	}
	if rec.TargetID != saved.ID {
		t.Fatalf("target_id: got %q want %q", rec.TargetID, saved.ID)
	}
	if got := rec.Metadata["role"]; got != "admin" {
		t.Fatalf("metadata role: got %v want admin", got)
	}
}

// app-events-audit-42 (role changed half): UpdateUserHandler emits
// user.role_changed with metadata {new_role} when the role actually changes.
func TestUpdateUserHandler_RecordsRoleChangedAudit(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit_role_changed.db"))
	target := fx.SeedUser(t, "bob", "secret12", "user")

	collector := &shared.AuditCollector{}
	ctx := shared.ContextWithAuditCollector(context.Background(), collector)

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	if err := h.Handle(ctx, UpdateUserCommand{
		ID:       target.ID,
		Nickname: target.Nickname,
		Email:    target.Email,
		Role:     "admin", // user -> admin
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	changed := auditEventsByAction(collector.Drain(), "user.role_changed")
	if len(changed) != 1 {
		t.Fatalf("user.role_changed records: got %d want 1", len(changed))
	}
	rec := changed[0]
	if rec.TargetType != "user" {
		t.Fatalf("target_type: got %q want user", rec.TargetType)
	}
	if rec.TargetID != target.ID {
		t.Fatalf("target_id: got %q want %q", rec.TargetID, target.ID)
	}
	if got := rec.Metadata["new_role"]; got != "admin" {
		t.Fatalf("metadata new_role: got %v want admin", got)
	}
}

// app-events-audit-42 (unchanged-role half): no user.role_changed event when
// the role is identical, even though the update otherwise succeeds.
func TestUpdateUserHandler_NoRoleChangedAuditWhenRoleUnchanged(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit_role_same.db"))
	target := fx.SeedUser(t, "bob", "secret12", "user")

	collector := &shared.AuditCollector{}
	ctx := shared.ContextWithAuditCollector(context.Background(), collector)

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	// Change only the email; role stays "user".
	if err := h.Handle(ctx, UpdateUserCommand{
		ID:       target.ID,
		Nickname: target.Nickname,
		Email:    "bob+changed@example.com",
		Role:     "user",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Confirm the update actually took effect (so this isn't asserting a no-op).
	updated, err := fx.Users.FindByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if updated.Email != "bob+changed@example.com" {
		t.Fatalf("email not updated: got %q", updated.Email)
	}

	if changed := auditEventsByAction(collector.Drain(), "user.role_changed"); len(changed) != 0 {
		t.Fatalf("expected no user.role_changed records, got %d", len(changed))
	}
}

// app-events-audit-43: DeleteUserHandler emits user.deleted (no metadata).
func TestDeleteUserHandler_RecordsUserDeletedAudit(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit_user_deleted.db"))
	admin := fx.SeedUser(t, "admin", "secret12", "admin")
	target := fx.SeedUser(t, "bob", "secret12", "user")

	collector := &shared.AuditCollector{}
	ctx := shared.ContextWithAuditCollector(authedCtx(admin.ID, "admin"), collector)

	h := NewDeleteUserHandler(fx.Users)
	if err := h.Handle(ctx, DeleteUserCommand{ID: target.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	deleted := auditEventsByAction(collector.Drain(), "user.deleted")
	if len(deleted) != 1 {
		t.Fatalf("user.deleted records: got %d want 1", len(deleted))
	}
	rec := deleted[0]
	if rec.TargetType != "user" {
		t.Fatalf("target_type: got %q want user", rec.TargetType)
	}
	if rec.TargetID != target.ID {
		t.Fatalf("target_id: got %q want %q", rec.TargetID, target.ID)
	}
	if rec.Metadata != nil {
		t.Fatalf("expected nil metadata, got %v", rec.Metadata)
	}
}

// guide-forms-fe-05: value objects are constructed in sequence and the FIRST
// ValidationError wins. With both an invalid nickname AND an invalid role,
// NewNickname runs before NewRole, so the nickname error must be returned.
func TestCreateUserHandler_FirstValidationErrorWins(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit_first_error.db"))

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(context.Background(), CreateUserCommand{
		Nickname: "", // invalid: nickname required
		Password: "secret12",
		Email:    "bob@example.com",
		Role:     "superhero", // also invalid, but checked AFTER nickname
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "nickname" {
		t.Fatalf("first error should be nickname, got field=%q", ve.Field)
	}
}

// guide-forms-fe-06: duplicate nickname returns *shared.ValidationError with
// Field="nickname" AND the exact message. (Existing TestCreateUserHandler_
// DuplicateNickname checks the Field but not the Message.)
func TestCreateUserHandler_DuplicateNicknameMessage(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit_dup_msg.db"))
	fx.SeedUser(t, "alice", "secret12", "user")

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(context.Background(), CreateUserCommand{
		Nickname: "alice",
		Password: "secret12",
		Email:    "alice2@example.com",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "nickname" {
		t.Fatalf("field: got %q want nickname", ve.Field)
	}
	if ve.Message != "user with this nickname already exists" {
		t.Fatalf("message: got %q want %q", ve.Message, "user with this nickname already exists")
	}
}
