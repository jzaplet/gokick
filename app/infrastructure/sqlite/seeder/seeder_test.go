package seeder_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/sqlite/seeder"
	"gokick/app/internal/testfx"
)

func newSeeder(t *testing.T, fx *testfx.Fixture, pw seeder.SeedAdminPassword) *seeder.Seeder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Single-tenant (multitenant=false) — admin lands in the default tenant.
	return seeder.NewSeeder(fx.Users, fx.Tenants, fx.Hasher, pw, "", "Tenant 1", false, logger)
}

// newSeederSuper builds a seeder with both the admin and the (optional)
// superadmin password set, for the superadmin-seed tests (single-tenant).
func newSeederSuper(
	t *testing.T,
	fx *testfx.Fixture,
	adminPw seeder.SeedAdminPassword,
	superPw seeder.SeedSuperAdminPassword,
) *seeder.Seeder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return seeder.NewSeeder(
		fx.Users,
		fx.Tenants,
		fx.Hasher,
		adminPw,
		superPw,
		"Tenant 1",
		false,
		logger,
	)
}

// newSeederMT builds a MULTITENANT seeder with the given admin-tenant name.
func newSeederMT(
	t *testing.T,
	fx *testfx.Fixture,
	adminPw seeder.SeedAdminPassword,
	superPw seeder.SeedSuperAdminPassword,
	tenantName seeder.SeedAdminTenant,
) *seeder.Seeder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return seeder.NewSeeder(
		fx.Users,
		fx.Tenants,
		fx.Hasher,
		adminPw,
		superPw,
		tenantName,
		true,
		logger,
	)
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

// Single-tenant (multitenancy off): the seeded admin lands in the DEFAULT tenant
// — the deployment behaves as if tenants did not exist.
func TestSeeder_SingleTenant_AdminInDefaultTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_single_tenant.db"))

	if err := newSeeder(t, fx, "valid-password-12").Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin, _ := fx.Users.FindByNickname(ctx, "admin")
	if admin == nil {
		t.Fatal("admin must exist")
	}
	if admin.TenantID != shared.DefaultTenantID {
		t.Fatalf("single-tenant admin must be in the default tenant, got %q", admin.TenantID)
	}
}

// Multitenant: the seeded admin gets its OWN named tenant (not the default), the
// superadmin still lives in the default tenant, and re-seeding is idempotent (no
// second tenant, no error).
func TestSeeder_Multitenant_AdminGetsOwnTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_multitenant.db"))

	if err := newSeederMT(t, fx, "valid-password-12", "super-password-12", "Acme").Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin, _ := fx.Users.FindByNickname(ctx, "admin")
	if admin == nil {
		t.Fatal("admin must exist")
	}
	if admin.TenantID == shared.DefaultTenantID {
		t.Fatal("multitenant admin must NOT be in the default tenant")
	}

	adminTenant, err := fx.Tenants.FindByID(ctx, admin.TenantID)
	if err != nil {
		t.Fatalf("load admin tenant: %v", err)
	}
	if adminTenant == nil || adminTenant.Name != "Acme" {
		t.Fatalf("admin must land in the named tenant 'Acme', got %+v", adminTenant)
	}

	// Superadmin stays in the default tenant even under multitenancy.
	super, _ := fx.Users.FindByNickname(ctx, "superadmin")
	if super == nil || super.TenantID != shared.DefaultTenantID {
		t.Fatalf("superadmin must stay in the default tenant, got %+v", super)
	}

	// Idempotent: a second seed must not create a second 'Acme' tenant.
	if err := newSeederMT(t, fx, "valid-password-12", "super-password-12", "Acme").Seed(ctx); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	var acmeCount int
	if err := fx.DB.DB().GetContext(ctx, &acmeCount,
		`SELECT COUNT(*) FROM tenants WHERE name='Acme'`); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if acmeCount != 1 {
		t.Fatalf("re-seed must not duplicate the admin tenant, got %d 'Acme' tenants", acmeCount)
	}
}

// Seed dispatched through the SystemCommandBus persists the bootstrap audit trail:
// the seeder's Record calls drain through AuditMiddleware in a single tx. MT +
// superadmin → two user.created (admin, superadmin) and one tenant.created (the
// admin's own tenant). Outside the bus (the other tests) the collector is a no-op.
func TestSeeder_AuditTrailThroughSystemBus(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "seed_audit.db"))
	s := newSeederMT(t, fx, "valid-password-12", "super-password-12", "Acme")

	err := bus.SystemDispatchVoid(context.Background(), fx.NewSystemBus(), "Seed", struct{}{},
		func(ctx context.Context) error { return s.Seed(ctx) })
	if err != nil {
		t.Fatalf("seed through bus: %v", err)
	}

	assertAuditCount(t, fx, "user.created", 2)
	assertAuditCount(t, fx, "tenant.created", 1)
}

func assertAuditCount(t *testing.T, fx *testfx.Fixture, action string, want int) {
	t.Helper()
	var n int
	if err := fx.DB.DB().GetContext(context.Background(), &n,
		`SELECT COUNT(*) FROM audit_log WHERE action=?`, action); err != nil {
		t.Fatalf("audit count %q: %v", action, err)
	}
	if n != want {
		t.Fatalf("audit_log %q: got %d want %d", action, n, want)
	}
}
