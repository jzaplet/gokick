package seeder

import (
	"context"
	"fmt"
	"log/slog"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"

	"github.com/google/uuid"
)

// logKeyNickname is the seeder's only structured-log key. sloglint's
// no-raw-keys forbids bare string keys.
const logKeyNickname = "nickname"

// SeedAdminPassword is a Wire-distinct alias so the DI graph can bind the
// admin-seed password without colliding with other string values.
type SeedAdminPassword string

// SeedSuperAdminPassword is the Wire-distinct alias for the OPTIONAL superadmin
// seed password (Krok 4c). Empty = do not seed a superadmin.
type SeedSuperAdminPassword string

type Seeder struct {
	users              user.Repository
	hasher             shared.PasswordHasher
	adminPassword      SeedAdminPassword
	superAdminPassword SeedSuperAdminPassword
	logger             *slog.Logger
}

func NewSeeder(
	users user.Repository,
	hasher shared.PasswordHasher,
	adminPassword SeedAdminPassword,
	superAdminPassword SeedSuperAdminPassword,
	logger *slog.Logger,
) *Seeder {
	return &Seeder{
		users:              users,
		hasher:             hasher,
		adminPassword:      adminPassword,
		superAdminPassword: superAdminPassword,
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

	admin := &user.User{
		ID:           uuid.New().String(),
		Nickname:     "admin",
		PasswordHash: hash,
		Email:        "admin@localhost",
		Role:         string(user.RoleAdmin),
		TenantID:     shared.DefaultTenantID,
		Active:       true,
	}

	if err := s.users.Save(ctx, admin); err != nil {
		return err
	}

	s.logger.Info("seeded default admin user", logKeyNickname, "admin")
	return nil
}

// seedSuperAdmin mints the platform-plane account. Unlike the admin, it is
// OPTIONAL: an empty password means the deployment runs without a superadmin
// (the platform overview stays unreachable), so it is skipped silently rather
// than erroring. Lives in the default tenant — platform queries are
// cross-tenant, so its own tenant is immaterial.
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

	s.logger.Info("seeded superadmin user", logKeyNickname, "superadmin")
	return nil
}
