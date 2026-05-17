package audit_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/sqlite/audit"
	"gokick/app/internal/testfx"

	"github.com/google/uuid"
)

func newRepo(t *testing.T) (*audit.Repository, *testfx.Fixture) {
	t.Helper()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "audit.db"))
	return audit.NewRepository(fx.DB), fx
}

func TestRepository_SavePersistsAllFields(t *testing.T) {
	ctx := context.Background()
	r, fx := newRepo(t)

	actorID := "u-1"
	actorIP := "192.0.2.5"
	targetType := "user"
	targetID := "u-2"
	rec := &shared.AuditRecord{
		ID:          uuid.New().String(),
		ActorUserID: &actorID,
		ActorIP:     &actorIP,
		Action:      "user.created",
		TargetType:  &targetType,
		TargetID:    &targetID,
		Metadata:    []byte(`{"role":"admin"}`),
		CreatedAt:   time.Now(),
	}
	if err := r.Save(ctx, rec); err != nil {
		t.Fatalf("save: %v", err)
	}

	var got struct {
		Action      string  `db:"action"`
		ActorUserID *string `db:"actor_user_id"`
		ActorIP     *string `db:"actor_ip"`
		Metadata    []byte  `db:"metadata"`
	}
	if err := fx.DB.DB().GetContext(ctx, &got,
		`SELECT action, actor_user_id, actor_ip, metadata FROM audit_log WHERE id=?`, rec.ID); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Action != "user.created" {
		t.Fatalf("action: %q", got.Action)
	}
	if got.ActorUserID == nil || *got.ActorUserID != actorID {
		t.Fatalf("actor: %v", got.ActorUserID)
	}
	if string(got.Metadata) != `{"role":"admin"}` {
		t.Fatalf("metadata: %s", got.Metadata)
	}
}

func TestRepository_SaveWithNilOptionalFields(t *testing.T) {
	ctx := context.Background()
	r, fx := newRepo(t)

	// e.g. auth.login.failed for an unknown nickname: no actor, no target.
	rec := &shared.AuditRecord{
		ID:        uuid.New().String(),
		Action:    "auth.login.failed",
		CreatedAt: time.Now(),
	}
	if err := r.Save(ctx, rec); err != nil {
		t.Fatalf("save: %v", err)
	}

	var count int
	if err := fx.DB.DB().GetContext(ctx, &count,
		`SELECT COUNT(*) FROM audit_log WHERE actor_user_id IS NULL AND target_id IS NULL`); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 nullable-row, got %d", count)
	}
}
