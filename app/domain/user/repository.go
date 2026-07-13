package user

import (
	"context"
	"time"
)

type Repository interface {
	Save(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	// UpdatePassword sets a user's OWN password hash (self-service change-password).
	// Scoped to WHERE id=? with no role != 'superadmin' filter, so a superadmin can
	// change their own password (Update excludes superadmin rows to block a tenant
	// admin editing OTHERS — a self password change can't escalate). Errors on 0 rows.
	UpdatePassword(ctx context.Context, userID, passwordHash string, updatedAt time.Time) error
	Delete(ctx context.Context, id string) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByNickname(ctx context.Context, nickname string) (*User, error)
	FindAll(ctx context.Context) ([]User, error)

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

// PlatformRepository is the cross-tenant SUPERSET of Repository, used ONLY by the
// superadmin platform plane (application/platform/**). It adds the *AcrossTenants
// escape hatches that DELIBERATELY bypass tenant scoping.
//
// Keeping them off the everyday Repository is least privilege: a non-platform
// handler holds Repository and therefore cannot even name these methods, so an
// accidental cross-tenant read/write is a compile error — not something only code
// review can catch. The boundary is also asserted statically by
// app/application/zz_platform_isolation_test.go. The same concrete SQLite repo
// implements both interfaces; only platform handlers depend on this port.
type PlatformRepository interface {
	Repository

	// FindAllAcrossTenants returns every user (joined to its tenant name)
	// regardless of tenant — the deliberate INVERSE of FindAll's tenant scoping.
	// The query carries a tenant-scope-exempt marker so the conformance gate
	// admits it consciously.
	FindAllAcrossTenants(ctx context.Context) ([]PlatformRow, error)

	// CountAcrossTenants counts all users across all tenants (platform dashboard).
	CountAcrossTenants(ctx context.Context) (int, error)

	// UpdateAcrossTenants / DeleteAcrossTenants are the platform-plane writes: a
	// superadmin manages a user in ANY tenant. No tenant filter, but superadmin
	// rows are excluded so a platform account can never be edited/deleted via API.
	UpdateAcrossTenants(ctx context.Context, user *User) error
	DeleteAcrossTenants(ctx context.Context, id string) error
}
