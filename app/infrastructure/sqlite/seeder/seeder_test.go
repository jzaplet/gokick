package seeder_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"gokick/app/infrastructure/sqlite/seeder"
	"gokick/app/internal/testfx"
)

func newSeeder(t *testing.T, fx *testfx.Fixture, pw seeder.SeedAdminPassword) *seeder.Seeder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return seeder.NewSeeder(fx.Users, fx.Hasher, pw, "", logger)
}

// newSeederSuper builds a seeder with both the admin and the (optional)
// superadmin password set, for the superadmin-seed tests.
func newSeederSuper(
	t *testing.T,
	fx *testfx.Fixture,
	adminPw seeder.SeedAdminPassword,
	superPw seeder.SeedSuperAdminPassword,
) *seeder.Seeder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return seeder.NewSeeder(fx.Users, fx.Hasher, adminPw, superPw, logger)
}

func TestSeeder_RejectsEmptyPassword(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_empty.db"))
	err := newSeeder(t, fx, "").Seed(context.Background())
	if err == nil {
		t.Fatal("empty APP_SEED_ADMIN_PASSWORD must reject seed")
	}
	if !strings.Contains(err.Error(), "APP_SEED_ADMIN_PASSWORD") {
		t.Fatalf("error must name the offending env var, got %q", err)
	}
}

func TestSeeder_RejectsTooShortPassword(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_short.db"))
	err := newSeeder(t, fx, "short").Seed(context.Background())
	if err == nil {
		t.Fatal("password shorter than NewPassword's policy must be rejected")
	}
}

func TestSeeder_CreatesAdminWithValidPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_ok.db"))

	if err := newSeeder(t, fx, "valid-password-12").Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin, _ := fx.Users.FindByNickname(ctx, "admin")
	if admin == nil {
		t.Fatal("admin user must exist after seed")
	}
	if admin.Role != "admin" {
		t.Fatalf("role: got %s want admin", admin.Role)
	}
}

// Idempotency matters because `./bin/app seed` may be re-run during
// deploys; rerunning without APP_SEED_ADMIN_PASSWORD in env must not fail
// once the admin already exists.
func TestSeeder_IdempotentWhenAdminExists(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_repeat.db"))

	if err := newSeeder(t, fx, "valid-password-12").Seed(ctx); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	if err := newSeeder(t, fx, "").Seed(ctx); err != nil {
		t.Fatalf("second seed (empty pw, admin already present): %v", err)
	}
}

// An empty APP_SEED_SUPERADMIN_PASSWORD must NOT seed a superadmin
// (the platform plane is opt-in), and must NOT error — the admin still seeds.
func TestSeeder_SkipsSuperAdminWhenPasswordEmpty(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_no_super.db"))

	if err := newSeederSuper(t, fx, "valid-password-12", "").Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if admin, _ := fx.Users.FindByNickname(ctx, "admin"); admin == nil {
		t.Fatal("admin must still seed when superadmin password is empty")
	}
	if super, _ := fx.Users.FindByNickname(ctx, "superadmin"); super != nil {
		t.Fatal("superadmin must NOT be seeded when its password is empty")
	}
}

// A set APP_SEED_SUPERADMIN_PASSWORD seeds a superadmin account, and
// re-running the seed is idempotent.
func TestSeeder_SeedsSuperAdminWhenPasswordSet(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_super.db"))

	if err := newSeederSuper(t, fx, "valid-password-12", "super-password-12").Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	super, _ := fx.Users.FindByNickname(ctx, "superadmin")
	if super == nil {
		t.Fatal("superadmin must exist after seed with a password set")
	}
	if super.Role != "superadmin" {
		t.Fatalf("role: got %q want superadmin", super.Role)
	}

	// Idempotent: a second run must not fail or duplicate.
	if err := newSeederSuper(t, fx, "valid-password-12", "super-password-12").Seed(ctx); err != nil {
		t.Fatalf("second seed (idempotent): %v", err)
	}
}

// A set-but-invalid superadmin password is rejected, naming the env var.
func TestSeeder_RejectsInvalidSuperAdminPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_super_bad.db"))

	err := newSeederSuper(t, fx, "valid-password-12", "short").Seed(ctx)
	if err == nil {
		t.Fatal("a set-but-too-short superadmin password must be rejected")
	}
	if !strings.Contains(err.Error(), "APP_SEED_SUPERADMIN_PASSWORD") {
		t.Fatalf("error must name the offending env var, got %q", err)
	}
}
