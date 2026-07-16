package shared

// SortDirection is the cross-context sort direction for paged list reads.
// It lives in shared because bounded contexts must not import each other —
// user and tenant grids both take one. Wire values map through
// SortDirectionFrom (whitelist with an ASC fallback), never raw into SQL.
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
