package audit_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"

	"github.com/google/uuid"
)

func newRepo(t *testing.T) (shared.AuditLogger, *testfx.Fixture) {
	t.Helper()
	fx := testfx.New(t)
	return fx.Audit, fx
}

func TestRepository_SavePersistsAllFields(t *testing.T) {
	ctx := context.Background()
	r, fx := newRepo(t)

	actorID := uuid.NewString()
	actorIP := "192.0.2.5"
	targetType := "user"
	targetID := uuid.NewString()
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

	got := fx.AuditEntry(t, rec.ID)
	if got.Action != "user.created" {
		t.Fatalf("action: %q", got.Action)
	}
	if got.ActorUserID == nil || *got.ActorUserID != actorID {
		t.Fatalf("actor: %v", got.ActorUserID)
	}
	if got.ActorIP == nil || *got.ActorIP != actorIP {
		t.Fatalf("actor ip: %v", got.ActorIP)
	}
	if got.TargetType == nil || *got.TargetType != targetType ||
		got.TargetID == nil || *got.TargetID != targetID {
		t.Fatalf("target: %v/%v", got.TargetType, got.TargetID)
	}
	// Compared as JSON values, not bytes: a database may store the document
	// normalized (Postgres jsonb re-spaces it).
	var gotMeta, wantMeta any
	if err := json.Unmarshal(got.Metadata, &gotMeta); err != nil {
		t.Fatalf("metadata is not JSON: %s", got.Metadata)
	}
	_ = json.Unmarshal(rec.Metadata, &wantMeta)
	if !reflect.DeepEqual(gotMeta, wantMeta) {
		t.Fatalf("metadata: got %s want %s", got.Metadata, rec.Metadata)
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

	count := fx.Count(t, "audit_log", "actor_user_id IS NULL AND target_id IS NULL")
	if count != 1 {
		t.Fatalf("expected 1 nullable-row, got %d", count)
	}
}
