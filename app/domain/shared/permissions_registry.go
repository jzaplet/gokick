package shared

import "sort"

// PermissionsRegistry holds the set of permissions known to the application.
// It is built at startup from every command/query handler that implements
// the Permissioned interface, giving a single source of truth derived
// directly from the code — no separate list to maintain.
type PermissionsRegistry struct {
	all []string
}

// NewPermissionsRegistry collects RequiredPermission() values from items,
// deduplicates and sorts them.
func NewPermissionsRegistry(items []Permissioned) *PermissionsRegistry {
	seen := map[string]struct{}{}
	all := []string{}

	for _, p := range items {
		perm := p.RequiredPermission()
		if _, exists := seen[perm]; exists {
			continue
		}
		seen[perm] = struct{}{}
		all = append(all, perm)
	}

	sort.Strings(all)

	return &PermissionsRegistry{all: all}
}

// All returns a copy of every registered permission, sorted alphabetically.
func (r *PermissionsRegistry) All() []string {
	result := make([]string, len(r.all))
	copy(result, r.all)

	return result
}

// ForRole returns the permissions available to the given role, filtered by
// IsPermissionAllowedForRole. Admin gets the full list.
func (r *PermissionsRegistry) ForRole(role string) []string {
	result := []string{}
	for _, p := range r.all {
		if IsPermissionAllowedForRole(p, role) {
			result = append(result, p)
		}
	}

	return result
}
