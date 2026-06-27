package user_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// Save writes the user's tenant verbatim, so in multitenant mode a handler must not
// persist a user into a tenant other than its active scope (a cross-tenant write).
// A system/seed path with no active scope is trusted (it stamps the tenant itself).
func TestSave_CrossTenantWrite_Rejected(t *testing.T) {
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "save_xtenant.db"))
	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	ctxA := shared.ContextWithTenantID(context.Background(), tenantA.ID)

	mkUser := func(nick, tenantID string) *user.User {
		t.Helper()
		hash, err := fx.Hasher.Hash("password123")
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		nn, err := user.NewNickname(nick)
		if err != nil {
			t.Fatalf("nickname: %v", err)
		}
		em, err := user.NewEmail(nick + "@example.com")
		if err != nil {
			t.Fatalf("email: %v", err)
		}
		role, err := user.NewRole("user")
		if err != nil {
			t.Fatalf("role: %v", err)
		}
		return user.NewUser(nn, hash, em, role, tenantID)
	}

	// Operating in tenant A, persisting a user stamped tenant B → rejected.
	if err := fx.Users.Save(ctxA, mkUser("mallory", tenantB.ID)); err == nil {
		t.Fatal("a cross-tenant Save (active A, row B) must be rejected")
	}

	// Same tenant as the active scope → allowed.
	if err := fx.Users.Save(ctxA, mkUser("alice", tenantA.ID)); err != nil {
		t.Fatalf("same-tenant Save must succeed, got %v", err)
	}

	// No active scope (system/seed path) → trusted even with an explicit tenant.
	if err := fx.Users.Save(context.Background(), mkUser("bob", tenantB.ID)); err != nil {
		t.Fatalf(
			"a no-scope Save must be trusted (the seed path stamps its own tenant), got %v",
			err,
		)
	}
}
