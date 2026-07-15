package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func actorCtx(id string) context.Context {
	return shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: id, Role: "admin", Nickname: "root",
	})
}

// Bulk delete by explicit ids removes the targets but NEVER the actor — the
// single-delete self-protection, generalized to the bulk path.
func TestBulkDeleteUsers_ByIDsExcludesActor(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "bulk_delete.db"))
	root := fx.SeedUser(t, "root", "pwd", "admin")
	alice := fx.SeedUser(t, "alice", "pwd", "user")
	bob := fx.SeedUser(t, "bob", "pwd", "user")

	h := NewBulkDeleteUsersHandler(fx.Users)
	err := h.Handle(actorCtx(root.ID), BulkDeleteUsersCommand{
		IDs: []string{alice.ID, bob.ID, root.ID},
	})
	if err != nil {
		t.Fatalf("bulk delete: %v", err)
	}

	remaining, err := fx.Users.FindAll(context.Background())
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != root.ID {
		t.Fatalf("want only the actor to survive, got %d rows", len(remaining))
	}
}

// All-filtered mode deletes exactly what the filter set matches (and still
// spares the actor even when the filters match them).
func TestBulkDeleteUsers_AllFilteredHonorsFilters(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "bulk_delete_filtered.db"))
	root := fx.SeedUser(t, "root", "pwd", "admin")
	fx.SeedUser(t, "alice", "pwd", "user")
	carol := fx.SeedUser(t, "carol", "pwd", "admin")

	h := NewBulkDeleteUsersHandler(fx.Users)
	err := h.Handle(actorCtx(root.ID), BulkDeleteUsersCommand{
		AllFiltered: true,
		Role:        "admin",
	})
	if err != nil {
		t.Fatalf("bulk delete filtered: %v", err)
	}

	remaining, err := fx.Users.FindAll(context.Background())
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("want alice + the actor to survive, got %d rows", len(remaining))
	}
	for _, u := range remaining {
		if u.ID == carol.ID {
			t.Fatal("carol matches the role=admin filter and must be deleted")
		}
	}
}

func TestBulkDeleteUsers_EmptySelectionIsValidationError(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "bulk_delete_empty.db"))
	root := fx.SeedUser(t, "root", "pwd", "admin")

	h := NewBulkDeleteUsersHandler(fx.Users)
	err := h.Handle(actorCtx(root.ID), BulkDeleteUsersCommand{})

	var verr *shared.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError for an empty selection, got %v", err)
	}
}

// Deactivate by ids flips active only for the targets; the actor stays
// untouched even when listed.
func TestBulkSetUsersActive_DeactivatesTargetsNotActor(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "bulk_active.db"))
	root := fx.SeedUser(t, "root", "pwd", "admin")
	alice := fx.SeedUser(t, "alice", "pwd", "user")

	h := NewBulkSetUsersActiveHandler(fx.Users)
	err := h.Handle(actorCtx(root.ID), BulkSetUsersActiveCommand{
		IDs:       []string{alice.ID, root.ID},
		SetActive: false,
	})
	if err != nil {
		t.Fatalf("bulk deactivate: %v", err)
	}

	got, err := fx.Users.FindByID(context.Background(), alice.ID)
	if err != nil || got == nil {
		t.Fatalf("find alice: %v", err)
	}
	if got.Active != false {
		t.Fatal("alice must be deactivated")
	}

	actor, err := fx.Users.FindByID(context.Background(), root.ID)
	if err != nil || actor == nil {
		t.Fatalf("find root: %v", err)
	}
	if actor.Active != true {
		t.Fatal("the actor must never be touched by a bulk operation")
	}
}
