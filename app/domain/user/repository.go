package user

import (
	"context"
	"time"
)

type Repository interface {
	Save(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByNickname(ctx context.Context, nickname string) (*User, error)
	FindAllActive(ctx context.Context) ([]User, error)
	FindAll(ctx context.Context) ([]User, error)

	// FindAllAcrossTenants returns every user (joined to its tenant name)
	// regardless of tenant — the deliberate INVERSE of FindAll's tenant scoping.
	// Reserved for the superadmin platform plane; the query carries a
	// tenant-scope-exempt marker so the conformance gate admits it consciously.
	FindAllAcrossTenants(ctx context.Context) ([]PlatformRow, error)

	// CountAcrossTenants counts all users across all tenants (platform dashboard).
	CountAcrossTenants(ctx context.Context) (int, error)

	// UpdateAcrossTenants / DeleteAcrossTenants are the platform-plane writes: a
	// superadmin manages a user in ANY tenant. No tenant filter, but superadmin
	// rows are excluded so a platform account can never be edited/deleted via API.
	UpdateAcrossTenants(ctx context.Context, user *User) error
	DeleteAcrossTenants(ctx context.Context, id string) error

	// RecordLogin stamps last_login_at = now for the user on a successful login.
	// Best-effort analytics; runs OUTSIDE the caller's tx (raw pool) like
	// ResetFailedLogin, so it neither blocks login nor depends on the bus commit.
	RecordLogin(ctx context.Context, userID string) error

	// RecordFailedLogin atomically bumps the failed-login counter for the
	// user. The implementation decides reset / lock inside a single SQL
	// statement so two concurrent failed logins can't race past the
	// threshold. Returns the new locked_until when locking was triggered
	// (else nil). Must run OUTSIDE the caller's transaction so the count
	// persists when the login handler returns AuthError and the
	// surrounding bus tx rolls back.
	RecordFailedLogin(
		ctx context.Context,
		userID string,
		threshold int,
		window, lockDuration time.Duration,
	) (*time.Time, error)

	// ResetFailedLogin clears the counter after a successful login. Runs
	// outside the caller's tx for the same reason as RecordFailedLogin.
	ResetFailedLogin(ctx context.Context, userID string) error
}
