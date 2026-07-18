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
	// FindByID returns (nil, nil) when no user has that id — the same not-found
	// idiom as FindByNickname and the token/run/tenant ports. It is NOT the
	// handler's job to distinguish not-found via an error type: a nil user IS the
	// not-found signal, and any non-nil error is a genuine (transient) failure to
	// propagate. Each caller maps a nil user to its own response (400 for an admin
	// editing a stale id, 401/force-logout for a vanished authenticated user).
	FindByID(ctx context.Context, id string) (*User, error)
	// FindScopedByID is the tenant-SCOPED by-id read behind the admin read-one
	// endpoint (GET /admin/users/{id}). Unlike FindByID — an identity load that is
	// tenant-exempt on purpose (auth JWT subject, cross-tenant by design) — this
	// scopes WHERE tenant_id=? AND role != 'superadmin', exactly like FindAll: an
	// admin may read only a non-superadmin user in its OWN tenant. Returns
	// (nil, nil) when no such row exists (absent, other tenant, or a superadmin).
	FindScopedByID(ctx context.Context, id string) (*User, error)
	FindByNickname(ctx context.Context, nickname string) (*User, error)

	// FindAll has NO production caller by design — the users grid reads through
	// FindPage. It survives as the tenant-isolation probe (tenant_isolation_test,
	// superadmin_guard_test, bulk_users_test): "every row this tenant can see",
	// unpaged. That shape is the point — FindPage clamps PerPage at
	// ListPerPageMax, so a probe built on it could only ever prove "no leak in
	// the first page", and a probe must be dumber than the code it audits. It
	// stays on the PORT, not on the sqlite struct, so every adapter (the planned
	// Postgres one included) has to prove isolation. Do not delete it as dead
	// code; do not "optimize" it into FindPage.
	FindAll(ctx context.Context) ([]User, error)

	// FindPage is the paged/filtered/sorted admin list read behind the users
	// grid — same tenant scoping and superadmin exclusion as FindAll. Criteria
	// arrive pre-normalized (whitelisted sort, clamped paging); the returned
	// page carries the filtered total for the client-side pager.
	FindPage(ctx context.Context, criteria ListCriteria) (ListPage, error)

	// CountByActive returns the tenant-scoped total and active (non-superadmin)
	// user counts in ONE query — the admin dashboard stat, instead of abusing
	// the grid page read (SELECT * LIMIT 1) as a counter.
	CountByActive(ctx context.Context) (total, active int, err error)

	// BulkDelete removes every selected non-superadmin user in the caller's
	// tenant EXCEPT the selection's ExcludeID (the acting admin) and returns the
	// affected count. Selection is dual-mode: explicit ids, or the filter set
	// when AllFiltered is on.
	BulkDelete(ctx context.Context, sel BulkSelection) (int64, error)

	// BulkSetActive flips the active flag for the selection — same scoping and
	// self-exclusion as BulkDelete.
	BulkSetActive(ctx context.Context, sel BulkSelection, active bool) (int64, error)

	// RecordLogin stamps last_login_at = now for the user on a successful login.
	// Best-effort analytics; raw pool like ResetFailedLogin (login runs outside
	// the bus tx by design), so it neither blocks login nor ties the stamp to
	// any caller's commit.
	RecordLogin(ctx context.Context, userID string) error

	// RecordFailedLogin atomically bumps the failed-login counter for the
	// user. The implementation decides reset / lock inside a single SQL
	// statement so two concurrent failed logins can't race past the
	// threshold. Returns the new locked_until when locking was triggered
	// (else nil). Must run OUTSIDE any caller transaction — login runs
	// outside the bus tx by design (SkipTransaction), and the count must
	// never be tied to a caller's commit: a future in-tx caller's
	// rollback must not erase it.
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

	// FindPageAcrossTenants is the paged/filtered/sorted platform users grid
	// read — FindPage's shape without the tenant scoping (every tenant, joined
	// to the tenant name). Criteria pre-normalized, filtered total included; the
	// query carries a tenant-scope-exempt marker so the conformance gate admits
	// it consciously.
	FindPageAcrossTenants(
		ctx context.Context,
		criteria PlatformListCriteria,
	) (PlatformListPage, error)

	// FindByIDAcrossTenants is the cross-tenant read-one behind the platform
	// read-one endpoint (GET /platform/users/{id}) — the by-id inverse of
	// FindAll's tenant scoping, joined to the tenant name. No tenant filter (a
	// superadmin reads any user in any tenant); returns (nil, nil) when absent.
	FindByIDAcrossTenants(ctx context.Context, id string) (*PlatformRow, error)

	// BulkDeleteAcrossTenants / BulkSetActiveAcrossTenants are the platform
	// twins of the tenant-scoped bulk writes: any tenant, superadmin rows and
	// the selection's ExcludeID always spared, affected count returned.
	BulkDeleteAcrossTenants(ctx context.Context, sel PlatformBulkSelection) (int64, error)
	BulkSetActiveAcrossTenants(
		ctx context.Context,
		sel PlatformBulkSelection,
		active bool,
	) (int64, error)

	// CountAcrossTenants counts all users across all tenants (platform dashboard).
	CountAcrossTenants(ctx context.Context) (int, error)

	// UpdateAcrossTenants / DeleteAcrossTenants are the platform-plane writes: a
	// superadmin manages a user in ANY tenant. No tenant filter, but superadmin
	// rows are excluded so a platform account can never be edited/deleted via API.
	UpdateAcrossTenants(ctx context.Context, user *User) error
	DeleteAcrossTenants(ctx context.Context, id string) error

	// SaveAcrossTenants inserts a user into the tenant the row itself names,
	// rather than the caller's active one. Save refuses that (AssertTenantScope:
	// in multitenant mode a row must not be born outside the active scope) — and
	// it is right to, because a superadmin's OWN tenant is the default one, so a
	// superadmin creating a user in tenant X is exactly the cross-tenant write
	// that guard exists to stop by default. This is the conscious exception, and
	// the *AcrossTenants name is what confines it: the isolation gate refuses any
	// caller outside application/platform.
	SaveAcrossTenants(ctx context.Context, user *User) error
}
