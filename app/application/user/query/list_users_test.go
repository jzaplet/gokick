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
	page, err := h.Handle(ctx, ListUsersQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(page.Items) != 3 || page.Total != 3 {
		t.Fatalf("count: got %d items / total %d, want 3 / 3", len(page.Items), page.Total)
	}

	// Default sort is nickname ASC (the pre-grid list order).
	got := []string{page.Items[0].Nickname, page.Items[1].Nickname, page.Items[2].Nickname}
	if got[0] != "alice" || got[1] != "bob" || got[2] != "carol" {
		t.Fatalf("order: got %v", got)
	}
}

func TestListUsersHandler_Empty(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "list_users_empty.db"))

	h := NewListUsersHandler(fx.Users)
	page, err := h.Handle(ctx, ListUsersQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("count: got %d items / total %d, want 0 / 0", len(page.Items), page.Total)
	}
}

// The grid state round-trips through the wire query: paging clamps, sort
// whitelists (unknown column falls back to nickname), filters narrow both the
// page and the total.
func TestListUsersHandler_PagingSortingFiltering(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "list_users_grid.db"))
	fx.SeedUser(t, "alice", "secret12", "user")
	fx.SeedUser(t, "bob", "secret12", "user")
	fx.SeedUser(t, "carol", "secret12", "admin")

	h := NewListUsersHandler(fx.Users)

	page, err := h.Handle(
		ctx,
		ListUsersQuery{Page: 2, PerPage: 2, SortBy: "nickname", SortDir: "ASC"},
	)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].Nickname != "carol" {
		t.Fatalf("page 2: got total %d, items %d", page.Total, len(page.Items))
	}

	page, err = h.Handle(ctx, ListUsersQuery{SortBy: "nickname", SortDir: "DESC"})
	if err != nil {
		t.Fatalf("desc: %v", err)
	}
	if page.Items[0].Nickname != "carol" {
		t.Fatalf("desc: first item %q, want carol", page.Items[0].Nickname)
	}

	page, err = h.Handle(ctx, ListUsersQuery{SortBy: "drop table users", SortDir: "boom"})
	if err != nil {
		t.Fatalf("hostile sort: %v", err)
	}
	if len(page.Items) != 3 || page.Items[0].Nickname != "alice" {
		t.Fatalf("hostile sort must fall back to nickname ASC, got %v", page.Items[0].Nickname)
	}

	page, err = h.Handle(ctx, ListUsersQuery{Nickname: "ali"})
	if err != nil {
		t.Fatalf("filter nickname: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Nickname != "alice" {
		t.Fatalf("filter nickname: got total %d", page.Total)
	}

	page, err = h.Handle(ctx, ListUsersQuery{Role: "admin"})
	if err != nil {
		t.Fatalf("filter role: %v", err)
	}
	if page.Total != 1 || page.Items[0].Nickname != "carol" {
		t.Fatalf("filter role: got total %d", page.Total)
	}
}

func TestListUsersQuery_RequiredPermission(t *testing.T) {
	if got := (ListUsersQuery{}).RequiredPermission(); got != "admin:users:read" {
		t.Fatalf("expected admin:users:read, got %q", got)
	}
}
