package di

import (
	"slices"
	"testing"
)

// F-037: CLI-only permissions must NOT leak into the FE-facing PermissionsRegistry.
// This is the OUTCOME guard — it exercises the REAL provider (every command wired
// in), so it fails if someone adds a CLI-only command without the shared.CLIOnly
// marker, or drops a marker. The mechanism (the filter itself) is covered by a
// unit test in domain/shared; this pins the concrete perms that must stay out.
func TestProvidePermissionsRegistry_ExcludesCLIOnlyPermissions(t *testing.T) {
	all := providePermissionsRegistry().All()

	// These come only from CLI-only commands (create-superadmin, create-tenant,
	// get-tenant) — no HTTP route, so the frontend can never invoke them.
	cliOnly := []string{
		"platform:users:create",
		"platform:tenants:create",
		"platform:tenants:read",
	}
	for _, perm := range cliOnly {
		if slices.Contains(all, perm) {
			t.Errorf("CLI-only permission %q leaked into the FE-facing registry: %v", perm, all)
		}
	}

	// Sanity: an HTTP-reachable permission IS present, so the registry isn't empty
	// by accident (which would make the exclusion assertion pass vacuously).
	if !slices.Contains(all, "platform:overview") {
		t.Fatalf("expected FE-facing permission %q in registry, got %v", "platform:overview", all)
	}
}
