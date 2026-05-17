package user

import (
	"database/sql"
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

	// Brute-force tracking. Mutated only via Repository's
	// RecordFailedLogin / ResetFailedLogin (which run outside the bus
	// transaction so the counter persists even when login returns
	// AuthError and the surrounding tx rolls back). sql.NullTime is
	// used (not *time.Time) because the SQLite driver writes/reads
	// these as TEXT — the standard sql.NullTime scanner handles both
	// the string-from-DB and the NULL case without a custom type.
	FailedLoginAttempts int          `db:"failed_login_attempts"`
	LastFailedLoginAt   sql.NullTime `db:"last_failed_login_at"`
	LockedUntil         sql.NullTime `db:"locked_until"`
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
