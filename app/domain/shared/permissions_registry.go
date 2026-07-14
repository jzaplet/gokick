package shared

import "sort"

// PermissionsRegistry holds the set of permissions known to the application. The
// permission STRINGS are code-derived (each is a command/query's own
// RequiredPermission), so they're never hand-copied. Enrollment, though — WHICH
// handlers feed the registry — IS a hand-maintained list (providePermissionsRegistry
// in di); a forgotten entry would silently blank that permission on the frontend,
// so a conformance gate (di.TestProvidePermissionsRegistry_EnrollsEveryFEFacingPermission)
// asserts every non-CLIOnly RequiredPermission is enrolled.
type PermissionsRegistry struct {
	all []string
}

// NewPermissionsRegistry collects RequiredPermission() values from items,
// deduplicates and sorts them. Items implementing CLIOnly are skipped — their
// permission is CLI/SystemCommandBus-only and must not reach the FE-facing list.
func NewPermissionsRegistry(items []Permissioned) *PermissionsRegistry {
	seen := map[string]struct{}{}
	all := []string{}

	for _, p := range items {
		if _, cliOnly := p.(CLIOnly); cliOnly {
			continue // CLI-only: enforced via SystemCommandBus, never FE-facing
		}
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
