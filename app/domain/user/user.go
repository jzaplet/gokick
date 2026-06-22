package user

import (
	"database/sql"
	"time"

	"gokick/app/domain/shared"

	"github.com/google/uuid"
)

type User struct {
	ID           string `db:"id"`
	Nickname     string `db:"nickname"`
	PasswordHash string `db:"password_hash"`
	Email        string `db:"email"`
	Role         string `db:"role"`
	// TenantID scopes the user to a tenant. The DB column is NOT NULL with a FK
	// to tenants(id) and no default, and the repo INSERT writes this field
	// explicitly. NewUser seeds it to shared.DefaultTenantID; CreateUserHandler
	// then overrides it with the caller's resolved tenant so an admin creates
	// users in their OWN tenant. In single-tenant mode every caller resolves to
	// DefaultTenantID, so the value is the same for everyone.
	TenantID  string    `db:"tenant_id"`
	Active    bool      `db:"active"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

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

	// LastLoginAt is stamped on each successful login via Repository.RecordLogin
	// (raw pool, best-effort, like the brute-force counters above). NULL until
	// the user has logged in at least once. Powers the superadmin platform
	// overview.
	LastLoginAt sql.NullTime `db:"last_login_at"`
}

func NewUser(nickname Nickname, passwordHash string, email Email, role Role) *User {
	return &User{
		ID:           uuid.New().String(),
		Nickname:     string(nickname),
		PasswordHash: passwordHash,
		Email:        string(email),
		Role:         string(role),
		TenantID:     shared.DefaultTenantID,
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}
