package seeder

import (
	"context"
	"fmt"
	"log/slog"

	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/domain/user"

	"github.com/google/uuid"
)

// Structured-log keys. sloglint's no-raw-keys forbids bare string keys.
const (
	logKeyNickname = "nickname"
	logKeyTenant   = "tenant"
)

// SeedAdminPassword is a Wire-distinct alias so the DI graph can bind the
// admin-seed password without colliding with other string values.
type SeedAdminPassword string

// SeedSuperAdminPassword is the Wire-distinct alias for the OPTIONAL superadmin
// seed password. Empty = do not seed a superadmin.
type SeedSuperAdminPassword string

// SeedAdminTenant is the Wire-distinct name of the tenant the admin is seeded
// into when multitenancy is on.
type SeedAdminTenant string

// Multitenant is the Wire-distinct multitenancy flag (mirrors config.Multitenancy)
// — it decides whether the seeded admin gets its own tenant or the default one.
type Multitenant bool

type Seeder struct {
	users              user.Repository
	tenants            tenant.Repository
	hasher             shared.PasswordHasher
	adminPassword      SeedAdminPassword
	superAdminPassword SeedSuperAdminPassword
	adminTenant        SeedAdminTenant
	multitenant        Multitenant
	logger             *slog.Logger
}

func NewSeeder(
	users user.Repository,
	tenants tenant.Repository,
	hasher shared.PasswordHasher,
	adminPassword SeedAdminPassword,
	superAdminPassword SeedSuperAdminPassword,
	adminTenant SeedAdminTenant,
	multitenant Multitenant,
	logger *slog.Logger,
) *Seeder {
	return &Seeder{
		users:              users,
		tenants:            tenants,
		hasher:             hasher,
		adminPassword:      adminPassword,
		superAdminPassword: superAdminPassword,
		adminTenant:        adminTenant,
		multitenant:        multitenant,
		logger:             logger,
	}
}

// Seed creates the default admin (required) and, when configured, the superadmin
// (optional). Both are idempotent and may be re-run during deploys.
func (s *Seeder) Seed(ctx context.Context) error {
	if err := s.seedAdmin(ctx); err != nil {
		return err
	}
	return s.seedSuperAdmin(ctx)
}

func (s *Seeder) seedAdmin(ctx context.Context) error {
	existing, err := s.users.FindByNickname(ctx, "admin")
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	// Force the operator to supply a real password instead of silently
	// minting an admin with a guessable one. NewPassword enforces the same
	// length policy as the user-facing CRUD.
	pw, err := user.NewPassword(string(s.adminPassword))
	if err != nil {
		return fmt.Errorf("APP_SEED_ADMIN_PASSWORD: %w", err)
	}

	hash, err := s.hasher.Hash(string(pw))
	if err != nil {
		return err
	}

	tenantID, err := s.adminTenantID(ctx)
	if err != nil {
		return err
	}

	admin := &user.User{
		ID:           uuid.New().String(),
		Nickname:     "admin",
		PasswordHash: hash,
		Email:        "admin@localhost",
		Role:         string(user.RoleAdmin),
		TenantID:     tenantID,
		Active:       true,
	}

	if err := s.users.Save(ctx, admin); err != nil {
		return err
	}

	// Through the SystemCommandBus (the seed command dispatches via it) this
	// persists the bootstrap audit trail; outside the bus the collector is a
	// throwaway (no-op). No actor — a system bootstrap has no human principal.
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.created",
		TargetType: "user",
		TargetID:   admin.ID,
		Metadata:   map[string]any{"role": admin.Role},
	})

	s.logger.Info("seeded default admin user", logKeyNickname, "admin")
	return nil
}

// adminTenantID resolves the tenant the seeded admin lands in. Single-tenant
// (multitenancy off): the default tenant, so the deployment behaves as before.
// Multitenant: the admin gets its OWN tenant (find-or-create by name) rather than
// silently sharing the default — otherwise the multitenant deployment would start
// with its first admin in the fallback tenant.
func (s *Seeder) adminTenantID(ctx context.Context) (string, error) {
	if !bool(s.multitenant) {
		return shared.DefaultTenantID, nil
	}

	name := string(s.adminTenant)
	existing, err := s.tenants.FindByName(ctx, name)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return existing.ID, nil
	}

	t := tenant.NewTenant(name)
	if err := s.tenants.Save(ctx, t); err != nil {
		return "", err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "tenant.created",
		TargetType: "tenant",
		TargetID:   t.ID,
		Metadata:   map[string]any{"name": t.Name},
	})

	s.logger.Info("seeded admin tenant", logKeyTenant, name)
	return t.ID, nil
}

// seedSuperAdmin mints the platform-plane account. Unlike the admin, it is
// OPTIONAL: an empty password means the deployment runs without a superadmin
// (the platform overview stays unreachable), so it is skipped silently rather
// than erroring. ALWAYS lives in the default tenant (even when multitenancy is
// on) — platform queries are cross-tenant, so its own tenant is immaterial.
func (s *Seeder) seedSuperAdmin(ctx context.Context) error {
	if s.superAdminPassword == "" {
		return nil
	}

	existing, err := s.users.FindByNickname(ctx, "superadmin")
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	pw, err := user.NewPassword(string(s.superAdminPassword))
	if err != nil {
		return fmt.Errorf("APP_SEED_SUPERADMIN_PASSWORD: %w", err)
	}

	hash, err := s.hasher.Hash(string(pw))
	if err != nil {
		return err
	}

	superAdmin := &user.User{
		ID:           uuid.New().String(),
		Nickname:     "superadmin",
		PasswordHash: hash,
		Email:        "superadmin@localhost",
		Role:         string(user.RoleSuperAdmin),
		TenantID:     shared.DefaultTenantID,
		Active:       true,
	}

	if err := s.users.Save(ctx, superAdmin); err != nil {
		return err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.created",
		TargetType: "user",
		TargetID:   superAdmin.ID,
		Metadata:   map[string]any{"role": superAdmin.Role},
	})

	s.logger.Info("seeded superadmin user", logKeyNickname, "superadmin")
	return nil
}
