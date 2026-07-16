package user

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

type SortDirection string

const (
	SortAsc  SortDirection = "ASC"
	SortDesc SortDirection = "DESC"
)

func SortDirectionFrom(v string) SortDirection {
	if SortDirection(v) == SortDesc {
		return SortDesc
	}
	return SortAsc
}

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
	SortDir SortDirection
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
