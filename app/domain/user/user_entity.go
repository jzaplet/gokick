package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           string
	Nickname     string
	PasswordHash string
	Email        string
	Role         string
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewUser(nickname Nickname, passwordHash string, email string, role Role) *User {
	return &User{
		ID:           uuid.New().String(),
		Nickname:     string(nickname),
		PasswordHash: passwordHash,
		Email:        email,
		Role:         string(role),
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}
