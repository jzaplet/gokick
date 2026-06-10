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

type Seeder struct {
	users         user.Repository
	hasher        shared.PasswordHasher
	adminPassword SeedAdminPassword
	logger        *slog.Logger
}

func NewSeeder(
	users user.Repository,
	hasher shared.PasswordHasher,
	adminPassword SeedAdminPassword,
	logger *slog.Logger,
) *Seeder {
	return &Seeder{
		users:         users,
		hasher:        hasher,
		adminPassword: adminPassword,
		logger:        logger,
	}
}

func (s *Seeder) Seed(ctx context.Context) error {
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
		Active:       true,
	}

	if err := s.users.Save(ctx, admin); err != nil {
		return err
	}

	s.logger.Info("seeded default admin user", logKeyNickname, "admin")
	return nil
}
