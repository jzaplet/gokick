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
	return c
}

func (c ListCriteria) Offset() int {
	return (c.Page - 1) * c.PerPage
}

type ListPage struct {
	Items []Overview
	Total int
}
