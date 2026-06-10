package seeder_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gokick/app/internal/testfx"
)

// TestSeeder_RejectsLiteralAdminPassword pins the hardened behavior that
// replaced the documented (but false) "admin:admin" default credential: the
// guessable literal password "admin" is 5 characters, below NewPassword's
// 8-char minimum, so the seeder MUST reject it and MUST NOT mint an admin row.
// This directly falsifies the doc claims that the seeder seeds password
// "admin" / credentials admin:admin (audit ids infra-db-security-16,
// overview-92). It is distinct from TestSeeder_RejectsTooShortPassword (which
// uses "short") because it pins the exact credential the docs promised.
func TestSeeder_RejectsLiteralAdminPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_admin_literal.db"))

	err := newSeeder(t, fx, "admin").Seed(ctx)
	if err == nil {
		t.Fatal("the guessable password \"admin\" must be rejected, not seeded")
	}
	if !strings.Contains(err.Error(), "APP_SEED_ADMIN_PASSWORD") {
		t.Fatalf("rejection must name the offending env var, got %q", err)
	}

	// No admin account may be created when the seed is rejected.
	admin, ferr := fx.Users.FindByNickname(ctx, "admin")
	if ferr != nil {
		t.Fatalf("lookup admin: %v", ferr)
	}
	if admin != nil {
		t.Fatal("no admin user must exist after a rejected seed")
	}
}

// TestSeeder_SeededAdminPasswordVerifies closes the true half of the claims:
// the seeder seeds a default admin account under nickname "admin" whose stored
// password hash actually verifies against the operator-supplied password (and
// rejects a wrong one). Existing TestSeeder_CreatesAdminWithValidPassword only
// asserts the role; this proves the seeded credential is genuinely usable,
// which is the substance behind "creates a default admin user".
func TestSeeder_SeededAdminPasswordVerifies(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_admin_verify.db"))

	const pw = "valid-password-12"
	if err := newSeeder(t, fx, pw).Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin, err := fx.Users.FindByNickname(ctx, "admin")
	if err != nil {
		t.Fatalf("lookup admin: %v", err)
	}
	if admin == nil {
		t.Fatal("admin user must exist after a successful seed")
	}
	if admin.Nickname != "admin" {
		t.Fatalf("nickname: got %q want %q", admin.Nickname, "admin")
	}

	// The supplied password must authenticate against the stored hash.
	if err := fx.Hasher.Verify(pw, admin.PasswordHash); err != nil {
		t.Fatalf("supplied password must verify against seeded hash: %v", err)
	}
	// A different password must NOT verify (guards against a no-op hasher).
	if err := fx.Hasher.Verify("wrong-password-99", admin.PasswordHash); err == nil {
		t.Fatal("a wrong password must not verify against the seeded hash")
	}
}
