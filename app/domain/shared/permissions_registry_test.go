package shared

import (
	"reflect"
	"testing"
)

// fakePermissioned implements the Permissioned interface for tests.
type fakePermissioned string

func (f fakePermissioned) RequiredPermission() string { return string(f) }

func TestPermissionsRegistry_AllIsSortedAndDeduplicated(t *testing.T) {
	reg := NewPermissionsRegistry([]Permissioned{
		fakePermissioned("profile:read"),
		fakePermissioned("admin:users:create"),
		fakePermissioned("profile:read"), // duplicate
		fakePermissioned("admin:users:delete"),
	})

	want := []string{
		"admin:users:create",
		"admin:users:delete",
		"profile:read",
	}
	if got := reg.All(); !reflect.DeepEqual(got, want) {
		t.Fatalf("All() mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestPermissionsRegistry_ForRole_AdminGetsEverything(t *testing.T) {
	reg := NewPermissionsRegistry([]Permissioned{
		fakePermissioned("admin:users:create"),
		fakePermissioned("admin:users:delete"),
		fakePermissioned("profile:read"),
		fakePermissioned("auth:logout"),
	})

	want := []string{
		"admin:users:create",
		"admin:users:delete",
		"auth:logout",
		"profile:read",
	}
	if got := reg.ForRole("admin"); !reflect.DeepEqual(got, want) {
		t.Fatalf("admin ForRole mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestPermissionsRegistry_ForRole_UserDeniesAdminPrefix(t *testing.T) {
	reg := NewPermissionsRegistry([]Permissioned{
		fakePermissioned("admin:users:create"),
		fakePermissioned("admin:users:delete"),
		fakePermissioned("profile:read"),
		fakePermissioned("profile:update"),
		fakePermissioned("auth:logout"),
	})

	want := []string{
		"auth:logout",
		"profile:read",
		"profile:update",
	}
	if got := reg.ForRole("user"); !reflect.DeepEqual(got, want) {
		t.Fatalf("user ForRole mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestPermissionsRegistry_ForRole_UnknownRoleSameAsUser(t *testing.T) {
	// Any non-admin role is treated identically: admin:* is filtered out.
	reg := NewPermissionsRegistry([]Permissioned{
		fakePermissioned("admin:users:create"),
		fakePermissioned("profile:read"),
	})

	got := reg.ForRole("guest")
	want := []string{"profile:read"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown role ForRole mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestPermissionsRegistry_EmptyInput(t *testing.T) {
	reg := NewPermissionsRegistry(nil)
	if got := reg.All(); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
	if got := reg.ForRole("admin"); len(got) != 0 {
		t.Fatalf("expected empty for admin, got %v", got)
	}
}

func TestPermissionsRegistry_AllReturnsCopy(t *testing.T) {
	// Mutating the returned slice must not affect the registry.
	reg := NewPermissionsRegistry([]Permissioned{
		fakePermissioned("profile:read"),
	})
	all := reg.All()
	all[0] = "tampered"

	if reg.All()[0] != "profile:read" {
		t.Fatal("internal state must not be mutable via All() result")
	}
}

func TestIsPermissionAllowedForRole(t *testing.T) {
	cases := []struct {
		name       string
		permission string
		role       string
		want       bool
	}{
		{"admin can do admin action", "admin:users:create", "admin", true},
		{"admin can do profile action", "profile:read", "admin", true},
		{"user cannot do admin action", "admin:users:create", "user", false},
		{"user can do profile action", "profile:read", "user", true},
		{"user can do auth:logout", "auth:logout", "user", true},
		{"guest cannot do admin action", "admin:anything", "guest", false},
		{"empty role cannot do admin action", "admin:x", "", false},
		{"empty role can do non-admin", "profile:read", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPermissionAllowedForRole(tc.permission, tc.role); got != tc.want {
				t.Fatalf("permission=%q role=%q: got %v want %v",
					tc.permission, tc.role, got, tc.want)
			}
		})
	}
}
