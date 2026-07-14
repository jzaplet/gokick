package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
)

type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, u *user.User) error {
	// Cross-tenant write guard: Save writes u.TenantID verbatim, so in multitenant
	// mode a row must not be placed in a tenant other than the active scope (the
	// platform plane uses the separate *AcrossTenants methods). System/seed paths
	// carry no scope and are trusted (AssertTenantScope skips them).
	if err := shared.AssertTenantScope(ctx, u.TenantID, r.Multitenancy()); err != nil {
		return err
	}
	const q = `INSERT INTO users (id, nickname, password_hash, email, role, tenant_id, active, created_at, updated_at)
		VALUES (:id, :nickname, :password_hash, :email, :role, :tenant_id, :active, :created_at, :updated_at)`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, u)
	return err
}

// Update scopes the WHERE to the caller's tenant (r.Tenant, positional — the
// guard is the CALLER's tenant, not the loaded row's) AND excludes superadmin
// rows: a tenant admin must never modify (e.g. reset the password of) a platform
// superadmin, even one that shares its tenant — that would be a back-door
// escalation. The platform account is managed out-of-band, never through
// tenant-admin user management.
func (r *Repository) Update(ctx context.Context, u *user.User) error {
	const q = `UPDATE users SET nickname=?, password_hash=?, email=?, role=?, active=?, updated_at=?
		WHERE id=? AND tenant_id=? AND role != 'superadmin'`
	res, err := r.Conn(ctx).ExecContext(ctx, q,
		u.Nickname, u.PasswordHash, u.Email, u.Role, u.Active, u.UpdatedAt, u.ID, r.Tenant(ctx))
	return requireOneRow(res, err)
}

// Delete scopes by tenant AND excludes superadmin rows — same rationale as Update.
func (r *Repository) Delete(ctx context.Context, id string) error {
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM users WHERE id=? AND tenant_id=? AND role != 'superadmin'`, id, r.Tenant(ctx))
	return requireOneRow(res, err)
}

// UpdatePassword sets a user's OWN password hash — the self-service write behind
// the profile change-password flow. The id is always the authenticated subject's
// own claims.UserID, so unlike Update it carries NO role != 'superadmin' filter: a
// superadmin changing their own password is legitimate and must not silently
// no-op. That escalation guard exists only to stop a tenant admin editing OTHER
// users; a self password change can't escalate. Scoped WHERE id=? — a self
// identity write, tenant-exempt like FindByID — and errors on 0 rows.
func (r *Repository) UpdatePassword(
	ctx context.Context,
	userID, passwordHash string,
	updatedAt time.Time,
) error {
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET password_hash=?, updated_at=? WHERE id=?
		 /* tenant-scope-exempt: self password change by id (subject == claims.UserID) */`,
		passwordHash, updatedAt, userID)
	return requireOneRow(res, err)
}

// FindByID loads the user named by an exact id — used by auth (the JWT subject)
// and by self/admin lookups. It is an identity load, not a tenant-filtered list,
// so it is exempt from tenant scoping (and runs before a tenant is known on the
// refresh path). CHOSEN 4a boundary: an admin CAN read another tenant's user by
// exact id. The enumerable list leak is closed (FindAll is tenant-scoped) and the
// mutate paths are scoped (Update/Delete), so a targeted by-id read is a known,
// smaller surface — not an oversight. Scope it too if that surface matters.
//
// Not-found returns (nil, nil) — the repository idiom every other lookup already
// follows (FindByNickname, token/run/tenant FindByID). Application handlers own
// the not-found *response*: each decides whether a missing user is a 400
// (admin edits a stale id), a 401 (the authenticated user's own row is gone), or
// a force-logout (refresh). See the contract on user.Repository.FindByID (F-011).
func (r *Repository) FindByID(ctx context.Context, id string) (*user.User, error) {
	var u user.User
	err := r.Conn(ctx).GetContext(ctx, &u,
		`SELECT * FROM users WHERE id=? /* tenant-scope-exempt: identity load by id */`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByNickname is the login lookup — it runs before any tenant is resolved,
// and nickname is globally unique, so it is a global identity lookup exempt
// from tenant scoping.
func (r *Repository) FindByNickname(ctx context.Context, nickname string) (*user.User, error) {
	var u user.User
	err := r.Conn(ctx).GetContext(ctx, &u,
		`SELECT * FROM users WHERE nickname=? /* tenant-scope-exempt: global identity lookup (login) */`,
		nickname)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func (r *Repository) FindAll(ctx context.Context) ([]user.User, error) {
	var users []user.User
	err := r.Conn(ctx).SelectContext(ctx, &users,
		`SELECT * FROM users WHERE tenant_id=? AND role != 'superadmin' ORDER BY nickname`,
		r.Tenant(ctx))
	return users, err
}

// FindAllAcrossTenants is the platform-plane read: every user, all tenants,
// joined to its tenant name — the deliberate inverse of FindAll. It does NOT call
// r.Tenant(ctx); the marker makes the cross-tenant scope explicit to the
// conformance gate. INNER JOIN is safe (tenant_id is a NOT NULL FK). Ordered by
// tenant then nickname so the superadmin list groups naturally.
func (r *Repository) FindAllAcrossTenants(ctx context.Context) ([]user.PlatformRow, error) {
	var rows []user.PlatformRow
	err := r.Conn(ctx).SelectContext(ctx, &rows,
		`SELECT u.id, u.nickname, u.email, u.role, u.active, u.tenant_id,
		        t.name AS tenant_name, u.last_login_at
		   FROM users u
		   JOIN tenants t ON t.id = u.tenant_id /* tenant-scope-exempt: platform superadmin */
		  ORDER BY t.name, u.nickname`)
	return rows, err
}

// CountAcrossTenants counts every user (all tenants, including superadmins) for
// the platform dashboard — it must match what the platform user list shows.
func (r *Repository) CountAcrossTenants(ctx context.Context) (int, error) {
	var n int
	err := r.Conn(ctx).GetContext(ctx, &n,
		`SELECT COUNT(*) FROM users /* tenant-scope-exempt: platform superadmin */`)
	return n, err
}

// UpdateAcrossTenants is the platform-plane write: a superadmin edits a user in
// ANY tenant. Unlike Update it carries no tenant filter (the marker makes that
// explicit), but it still excludes superadmin rows so no platform account can be
// edited through the API. tenant_id is deliberately NOT in the SET clause — an
// edit must never move a user between tenants.
func (r *Repository) UpdateAcrossTenants(ctx context.Context, u *user.User) error {
	const q = `UPDATE users SET nickname=?, password_hash=?, email=?, role=?, active=?, updated_at=?
		WHERE id=? AND role != 'superadmin' /* tenant-scope-exempt: platform superadmin */`
	res, err := r.Conn(ctx).ExecContext(ctx, q,
		u.Nickname, u.PasswordHash, u.Email, u.Role, u.Active, u.UpdatedAt, u.ID)
	return requireOneRow(res, err)
}

// DeleteAcrossTenants is the platform-plane delete — same cross-tenant scope and
// superadmin exclusion as UpdateAcrossTenants.
func (r *Repository) DeleteAcrossTenants(ctx context.Context, id string) error {
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM users WHERE id=? AND role != 'superadmin' /* tenant-scope-exempt: platform superadmin */`,
		id)
	return requireOneRow(res, err)
}

// RecordLogin stamps last_login_at on successful login. Raw pool (r.DB.DB()),
// outside the bus tx — same rationale as ResetFailedLogin: a successful login
// should record even if a later step in the handler rolls back. Best-effort.
func (r *Repository) RecordLogin(ctx context.Context, userID string) error {
	_, err := r.DB.DB().ExecContext(ctx,
		`UPDATE users SET last_login_at = strftime('%Y-%m-%d %H:%M:%f', 'now') WHERE id = ?
		 /* tenant-scope-exempt: stamp successful login by id */`,
		userID)
	return err
}

// RecordFailedLogin runs ENTIRELY in SQL so the counter decision (reset
// after the window, increment otherwise, lock when threshold reached) is
// atomic relative to other concurrent failed logins for the same row.
// Uses r.DB.DB() (raw pool) instead of r.Conn(ctx) (tx-aware) on purpose
// — the surrounding LoginHandler returns AuthError on bad credentials,
// which rolls back its bus transaction; the counter update must survive
// that rollback or brute-force protection becomes a no-op.
func (r *Repository) RecordFailedLogin(
	ctx context.Context,
	userID string,
	threshold int,
	window, lockDuration time.Duration,
) (*time.Time, error) {
	windowSec := int(window.Seconds())
	lockExpr := fmt.Sprintf("+%d seconds", int(lockDuration.Seconds()))

	// CASE branches read pre-update column values; `failed_login_attempts + 1`
	// is the post-increment count. Logic:
	//   - last attempt older than window  → reset to 1
	//   - else, increment by 1
	//   - if the resulting count hits the threshold → reset to 0 (so the
	//     post-unlock cycle starts fresh) AND set locked_until
	const q = `
		/* tenant-scope-exempt: brute-force counter, runs pre/at-login by user id */
		UPDATE users SET
		    failed_login_attempts = CASE
		        WHEN last_failed_login_at IS NULL
		             OR (julianday('now') - julianday(last_failed_login_at)) * 86400 > ?
		        THEN 1
		        WHEN failed_login_attempts + 1 >= ?
		        THEN 0
		        ELSE failed_login_attempts + 1
		    END,
		    last_failed_login_at = strftime('%Y-%m-%d %H:%M:%f', 'now'),
		    locked_until = CASE
		        WHEN last_failed_login_at IS NOT NULL
		             AND (julianday('now') - julianday(last_failed_login_at)) * 86400 <= ?
		             AND failed_login_attempts + 1 >= ?
		        THEN strftime('%Y-%m-%d %H:%M:%f', 'now', ?)
		        ELSE locked_until
		    END
		WHERE id = ?
		RETURNING locked_until`

	var locked sql.NullTime
	err := r.DB.DB().GetContext(ctx, &locked, q,
		windowSec, threshold, windowSec, threshold, lockExpr, userID)
	if err != nil {
		return nil, err
	}
	// Return locked_until ONLY when THIS attempt produced an ACTIVE lock (the port
	// contract: "the new lock, else nil"). The filter is in Go, not SQL, because a
	// RETURNING CASE expression loses the column's datetime affinity and comes back
	// as a raw string. RecordFailedLogin runs only when the account is NOT already
	// locked (login.go guards on the locked flag), so a FUTURE locked_until here can
	// only be one this attempt set; a stale PAST value left by an expired lock (the
	// ELSE branch keeps it) is NOT a fresh lock — returning it would be mis-read as
	// one and emit a phantom auth.account.locked audit event (F-045).
	if !locked.Valid || !locked.Time.After(time.Now()) {
		return nil, nil
	}
	return &locked.Time, nil
}

// ResetFailedLogin runs outside the caller's tx for the same reason as
// RecordFailedLogin — a successful login should clear the counter even
// if a later step in the same handler hits an error and rolls back.
func (r *Repository) ResetFailedLogin(ctx context.Context, userID string) error {
	_, err := r.DB.DB().ExecContext(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = ?
		 /* tenant-scope-exempt: clear brute-force counter on successful login */`,
		userID)
	return err
}

// requireOneRow turns a by-id mutation result into a not-found error when it
// matched no row. A scoped UPDATE/DELETE that changes 0 rows — target absent, or
// excluded by the WHERE guard (role != 'superadmin', wrong tenant) — must surface
// as an error, never a silent success: a silent no-op returns 204 AND lets the
// caller emit a phantom audit event for a write that never happened (F-023 phantom
// role_changed, F-039 superadmin self-password no-op). Unlike sqlite.RowsAffectedBool
// (owner-fencing, where 0 rows is a normal terminal outcome), here 0 rows is ALWAYS
// an error. Field "id" matches the not-found ValidationError the admin/platform
// handlers return on a missing user, so both map to the same 400.
func requireOneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return &shared.ValidationError{Field: "id", Message: "user not found"}
	}
	return nil
}
