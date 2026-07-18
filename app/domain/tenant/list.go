package tenant

import "gokick/app/domain/shared"

// List criteria for the platform tenants grid — same shape discipline as the
// user grids: grouped criteria (param gate), whitelisted sort, clamped paging.

type SortColumn string

const (
	SortByName  SortColumn = "name"
	SortByUsers SortColumn = "users"
)

func SortColumnFrom(v string) SortColumn {
	if SortColumn(v) == SortByUsers {
		return SortByUsers
	}
	return SortByName
}

type ListFilters struct {
	Name string
	// Plan filters by exact billing tier (the FE offers the known tiers in a
	// select); empty means all plans.
	Plan string
}

const (
	ListPerPageDefault = 25
	ListPerPageMax     = 100
)

type ListCriteria struct {
	Page    int
	PerPage int
	Sort    SortColumn
	SortDir shared.SortDirection
	Filters ListFilters
}

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
	// Clamp the direction (the column is whitelisted by overviewSortSQL; the
	// direction is interpolated into ORDER BY).
	c.SortDir = shared.SortDirectionFrom(string(c.SortDir))
	return c
}

func (c ListCriteria) Offset() int {
	return (c.Page - 1) * c.PerPage
}

type ListPage struct {
	Items []Overview
	Total int
}

// BulkSelection is what a bulk tenant operation acts on: either an explicit id
// list, or "every tenant matching the current filters" (AllFiltered) — the same
// two modes the user grids offer. There is no ExcludeID twin of
// user.PlatformBulkSelection: a tenant has no "self" to protect, and the two
// tenants that must survive (the default tenant, and any tenant still holding
// users) are refused by the delete itself rather than filtered out here.
type BulkSelection struct {
	IDs         []string
	AllFiltered bool
	Filters     ListFilters
}

func (s BulkSelection) IsEmpty() bool {
	return !s.AllFiltered && len(s.IDs) == 0
}
