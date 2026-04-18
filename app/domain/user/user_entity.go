package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           string    `db:"id"`
	Nickname     string    `db:"nickname"`
	PasswordHash string    `db:"password_hash"`
	Email        string    `db:"email"`
	Role         string    `db:"role"`
	Active       bool      `db:"active"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

func NewUser(nickname Nickname, passwordHash string, email Email, role Role) *User {
	return &User{
		ID:           uuid.New().String(),
		Nickname:     string(nickname),
		PasswordHash: passwordHash,
		Email:        string(email),
		Role:         string(role),
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}
