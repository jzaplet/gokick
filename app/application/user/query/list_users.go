package query

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// ListUsersQuery carries the grid state as it arrived on the wire (raw
// strings/ints); Handle normalizes it into whitelisted domain criteria. Sort
// and paging are UX preferences — unknown or out-of-range values fall back
// instead of erroring, so a stale deep link never 400s.
type ListUsersQuery struct {
	Page     int
	PerPage  int
	SortBy   string
	SortDir  string
	Nickname string
	Email    string
	Role     string
	Active   string
}

func (ListUsersQuery) RequiredPermission() string { return "admin:users:read" }

type ListUsersHandler struct {
	users user.Repository
}

func NewListUsersHandler(users user.Repository) *ListUsersHandler {
	return &ListUsersHandler{users: users}
}

func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsersQuery) (user.ListPage, error) {
	criteria := user.ListCriteria{
		Page:    q.Page,
		PerPage: q.PerPage,
		Sort:    user.SortColumnFrom(q.SortBy),
		SortDir: shared.SortDirectionFrom(q.SortDir),
		Filters: user.ListFilters{
			Nickname: q.Nickname,
			Email:    q.Email,
			Role:     q.Role,
			Active:   q.Active,
		},
	}.Normalize()

	return h.users.FindPage(ctx, criteria)
}
