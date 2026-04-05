package sqlite

import (
	"context"
	"log/slog"

	"myapp/app/domain/shared"
	"myapp/app/domain/user"

	"github.com/google/uuid"
)

type Seeder struct {
	users  user.Repository
	hasher shared.PasswordHasher
	logger *slog.Logger
}

func NewSeeder(users user.Repository, hasher shared.PasswordHasher, logger *slog.Logger) *Seeder {
	return &Seeder{users: users, hasher: hasher, logger: logger}
}

func (s *Seeder) Seed(ctx context.Context) error {
	existing, err := s.users.FindByNickname(ctx, "admin")
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	hash, err := s.hasher.Hash("admin")
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

	s.logger.Info("seeded default admin user", "nickname", "admin")
	return nil
}
