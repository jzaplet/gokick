package user

import "gokick/app/domain/shared"

// List criteria for the paged admin user list. The types live in the domain so
// the repository port can take ONE grouped criteria value (the param-count gate
// caps functions at 5 parameters on purpose — page/sort/filter sprawl belongs
// in a struct) and so sorting is a WHITELIST: the wire value maps onto an
// enumerated column or falls back, never onto raw SQL.

type SortColumn string

const (
	SortByNickname SortColumn = "nickname"
	SortByEmail    SortColumn = "email"
	SortByRole     SortColumn = "role"
)

// SortColumnFrom whitelists a wire value; anything unknown falls back to the
// nickname default (the pre-grid list order) instead of erroring — sort is a
// UX preference, not business input worth a 400.
func SortColumnFrom(v string) SortColumn {
	switch SortColumn(v) {
	case SortByEmail:
		return SortByEmail
	case SortByRole:
		return SortByRole
	case SortByNickname:
		return SortByNickname
	default:
		return SortByNickname
	}
}

// Sort direction lives in domain/shared — the tenant grid needs the same type
// and bounded contexts must not import each other.

// ListFilters are the admin-list filter values; empty string = filter off.
// Active is tri-state on purpose ("" all, "1" active, "0" inactive) — a bool
// cannot express "don't care".
type ListFilters struct {
	Nickname string
	Email    string
	Role     string
	Active   string
}

const (
	ListPerPageDefault = 25
	ListPerPageMax     = 100
)

// ListCriteria is the one grouped parameter the repository page read takes.
type ListCriteria struct {
	Page    int
	PerPage int
	Sort    SortColumn
	SortDir shared.SortDirection
	Filters ListFilters
}

// Normalize clamps paging to sane bounds instead of erroring: page/perPage are
// UX state (a stale deep link must not 400), and the max cap keeps a client
// from asking for the whole table in one page.
func (c ListCriteria) Normalize() ListCriteria {
	if c.Page < 1 {
		c.Page = 1
	}
	if c.PerPage < 1 {
		c.PerPage = ListPerPageDefault
	}
	if c.PerPage > ListPerPageMax {
		c.PerPage = ListPerPageMax
	}
	return c
}

func (c ListCriteria) Offset() int {
	return (c.Page - 1) * c.PerPage
}

// ListPage is what the paged read returns: one page of rows plus the total the
// filters match — the grid needs the total for its pager.
type ListPage struct {
	Items []User
	Total int
}

// Platform-plane list (the superadmin cross-tenant users grid) — a wider sort
// whitelist (tenant name, last login) and one extra filter (tenant name).

const (
	SortByTenant    SortColumn = "tenant"
	SortByLastLogin SortColumn = "last_login"
)

// PlatformSortColumnFrom whitelists the platform grid's sort; fallback is the
// tenant column (the pre-grid ORDER BY t.name, u.nickname order).
func PlatformSortColumnFrom(v string) SortColumn {
	switch SortColumn(v) {
	case SortByNickname, SortByEmail, SortByRole, SortByTenant, SortByLastLogin:
		return SortColumn(v)
	default:
		return SortByTenant
	}
}

type PlatformListFilters struct {
	ListFilters
	Tenant string
}

type PlatformListCriteria struct {
	Page    int
	PerPage int
	Sort    SortColumn
	SortDir shared.SortDirection
	Filters PlatformListFilters
}

func (c PlatformListCriteria) Normalize() PlatformListCriteria {
	if c.Page < 1 {
		c.Page = 1
	}
	if c.PerPage < 1 {
		c.PerPage = ListPerPageDefault
	}
	if c.PerPage > ListPerPageMax {
		c.PerPage = ListPerPageMax
	}
	return c
}

func (c PlatformListCriteria) Offset() int {
	return (c.Page - 1) * c.PerPage
}

type PlatformListPage struct {
	Items []PlatformRow
	Total int
}

// BulkSelection mirrors the grid's dual-mode selection: explicit ids, or
// "every row the filters match" WITHOUT enumerating ids (the client sends the
// filter set instead of a huge id list). ExcludeID is the acting admin — bulk
// operations never touch the actor, the same self-protection as single delete.
type BulkSelection struct {
	IDs         []string
	AllFiltered bool
	Filters     ListFilters
	ExcludeID   string
}

func (s BulkSelection) IsEmpty() bool {
	return !s.AllFiltered && len(s.IDs) == 0
}

// PlatformBulkSelection is BulkSelection's cross-tenant twin: the platform
// grid filters (incl. tenant name) instead of the tenant-scoped ones.
type PlatformBulkSelection struct {
	IDs         []string
	AllFiltered bool
	Filters     PlatformListFilters
	ExcludeID   string
}

func (s PlatformBulkSelection) IsEmpty() bool {
	return !s.AllFiltered && len(s.IDs) == 0
}
