package query

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/internal/testfx"
)

func TestListUsersHandler_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "list_users.db"))
	fx.SeedUser(t, "alice", "secret12", "user")
	fx.SeedUser(t, "bob", "secret12", "user")
	fx.SeedUser(t, "carol", "secret12", "admin")

	h := NewListUsersHandler(fx.Users)
	users, err := h.Handle(ctx, ListUsersQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(users) != 3 {
		t.Fatalf("count: got %d want 3", len(users))
	}

	// Repository orders by nickname ASC.
	if users[0].Nickname != "alice" || users[1].Nickname != "bob" || users[2].Nickname != "carol" {
		t.Fatalf("order: got %v", []string{users[0].Nickname, users[1].Nickname, users[2].Nickname})
	}
}

func TestListUsersHandler_Empty(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "list_users_empty.db"))

	h := NewListUsersHandler(fx.Users)
	users, err := h.Handle(ctx, ListUsersQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("count: got %d want 0", len(users))
	}
}

func TestListUsersQuery_RequiredPermission(t *testing.T) {
	if got := (ListUsersQuery{}).RequiredPermission(); got != "admin:users:read" {
		t.Fatalf("expected admin:users:read, got %q", got)
	}
}
