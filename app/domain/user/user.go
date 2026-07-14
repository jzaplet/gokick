package user

import (
	"time"

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
	// explicitly. NewUser REQUIRES it as a parameter (born scoped) so a user can
	// never be silently defaulted into the wrong tenant — enforced statically by
	// app/domain/zz_bornscoped_test.go. Single-tenant callers pass
	// shared.DefaultTenantID; an admin-created user gets the caller's resolved
	// tenant; a superadmin is homed in the default tenant explicitly.
	TenantID  string    `db:"tenant_id"`
	Active    bool      `db:"active"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	// Brute-force tracking. Mutated only via Repository's
	// RecordFailedLogin / ResetFailedLogin (which run outside the bus
	// transaction so the counter persists even when login returns
	// AuthError and the surrounding tx rolls back). *time.Time (not
	// sql.NullTime) for the nullable columns — the same shape run.Run
	// uses: the ncruces driver scans a nullable DATETIME TEXT column
	// into a *time.Time (nil for NULL) without a custom type, so nil is
	// the one "unset" sentinel across contexts. Writes stamp ms
	// precision (strftime %f), verified round-trip under load.
	FailedLoginAttempts int        `db:"failed_login_attempts"`
	LastFailedLoginAt   *time.Time `db:"last_failed_login_at"`
	LockedUntil         *time.Time `db:"locked_until"`

	// LastLoginAt is stamped on each successful login via Repository.RecordLogin
	// (raw pool, best-effort, like the brute-force counters above). nil until
	// the user has logged in at least once. Powers the superadmin platform
	// overview.
	LastLoginAt *time.Time `db:"last_login_at"`
}

func NewUser(
	nickname Nickname,
	passwordHash string,
	email Email,
	role Role,
	tenantID string,
) *User {
	return &User{
		ID:           uuid.Must(uuid.NewV7()).String(),
		Nickname:     string(nickname),
		PasswordHash: passwordHash,
		Email:        string(email),
		Role:         string(role),
		TenantID:     tenantID,
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}
