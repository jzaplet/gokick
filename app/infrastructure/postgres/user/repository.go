// Package user implements user.Repository on Postgres — the twin of the SQLite
// repository (same port contract, same scoping and superadmin floors), in native
// SQL: $n placeholders, statement_timestamp() for the database clock, ILIKE under
// the Czech collation. Ids are uuid columns, so an id from the outside is parsed
// first and a malformed one is the row that is not there (postgres.ParseID).
package user

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
)

type Repository struct {
	postgres.BaseRepository
}

func NewRepository(db *postgres.Manager) *Repository {
	return &Repository{BaseRepository: postgres.BaseRepository{DB: db}}
}

// nicknameKey is the unique constraint on users.nickname (Postgres names it
// <table>_<column>_key).
const nicknameKey = "users_nickname_key"

// nicknameTaken turns the unique violation on users.nickname into the error the
// handlers' own pre-check (FindByNickname) returns. The pre-check is for the
// message; the constraint is the truth: two concurrent creates of one nickname
// both pass the check — writes run in parallel here, unlike on SQLite — and the
// loser must get the same 400 its form expects, not a 500.
func nicknameTaken(err error) error {
	if postgres.IsUniqueViolation(err, nicknameKey) {
		return &shared.ValidationError{Field: "nickname", Key: msgkey.UserNicknameTaken}
	}
	return err
}

func (r *Repository) Save(ctx context.Context, u *user.User) error {
	// Cross-tenant write guard, as on SQLite; on the tenant plane row-level
	// security refuses a foreign tenant_id too (WITH CHECK).
	if err := shared.AssertTenantScope(ctx, u.TenantID, r.Multitenancy()); err != nil {
		return err
	}
	const q = `INSERT INTO users (id, nickname, password_hash, email, role, tenant_id, active, created_at, updated_at, lang)
		VALUES (:id, :nickname, :password_hash, :email, :role, :tenant_id, :active, :created_at, :updated_at, :lang)`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, u)
	return nicknameTaken(err)
}

// SaveAcrossTenants is Save without the scope guard — the platform create into
// the CHOSEN tenant. The statement is duplicated from Save on purpose (the
// write-side gate pairs each tenant_id-stamping INSERT with a guard in the same
// function), and the marker says "exempt - reason" because sqlx would bind a
// ": " inside a named query — see the SQLite twin.
func (r *Repository) SaveAcrossTenants(ctx context.Context, u *user.User) error {
	const q = `INSERT INTO users (id, nickname, password_hash, email, role, tenant_id, active, created_at, updated_at, lang)
		VALUES (:id, :nickname, :password_hash, :email, :role, :tenant_id, :active, :created_at, :updated_at, :lang)
		/* tenant-write-exempt - platform superadmin creates into the CHOSEN tenant */`
	_, err := r.SystemConn(ctx).NamedExecContext(ctx, q, u)
	return nicknameTaken(err)
}

// Update scopes to the caller's tenant and never touches a superadmin row — see
// the SQLite twin.
func (r *Repository) Update(ctx context.Context, u *user.User) error {
	id, ok := postgres.ParseID(u.ID)
	if !ok {
		return requireOneRow(nil, nil)
	}
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET nickname = $1, password_hash = $2, email = $3, role = $4, active = $5, updated_at = $6
		  WHERE id = $7 AND tenant_id = $8 AND role <> 'superadmin'`,
		u.Nickname, u.PasswordHash, u.Email, u.Role, u.Active, u.UpdatedAt, id, r.Tenant(ctx))
	return requireOneRow(res, nicknameTaken(err))
}

// Delete scopes by tenant AND excludes superadmin rows — same rationale as Update.
func (r *Repository) Delete(ctx context.Context, id string) error {
	uid, ok := postgres.ParseID(id)
	if !ok {
		return requireOneRow(nil, nil)
	}
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM users WHERE id = $1 AND tenant_id = $2 AND role <> 'superadmin'`,
		uid, r.Tenant(ctx))
	return requireOneRow(res, err)
}

// UpdatePassword sets a user's OWN password hash (subject == claims.UserID) —
// see the SQLite twin for why it carries no superadmin floor.
func (r *Repository) UpdatePassword(
	ctx context.Context,
	userID, passwordHash string,
	updatedAt time.Time,
) error {
	id, ok := postgres.ParseID(userID)
	if !ok {
		return requireOneRow(nil, nil)
	}
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3
		 /* tenant-scope-exempt: self password change by id (subject == claims.UserID) */`,
		passwordHash, updatedAt, id)
	return requireOneRow(res, err)
}

// UpdateLang sets the user's own UI-language preference — same self-service
// scoping as UpdatePassword.
func (r *Repository) UpdateLang(
	ctx context.Context,
	userID string,
	lang shared.Lang,
	updatedAt time.Time,
) error {
	id, ok := postgres.ParseID(userID)
	if !ok {
		return requireOneRow(nil, nil)
	}
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET lang = $1, updated_at = $2 WHERE id = $3
		 /* tenant-scope-exempt: self language change by id (subject == claims.UserID) */`,
		string(lang), updatedAt, id)
	return requireOneRow(res, err)
}

// FindByID loads the user named by an exact id (the JWT subject, the refresh
// path, self and admin lookups). Not-found is (nil, nil). The query itself is
// tenant-exempt, as on SQLite; on the tenant plane row-level security still
// limits it to the caller's tenant — the refresh path, which runs before a tenant
// is known, is on the system plane (shared.PreTenant).
func (r *Repository) FindByID(ctx context.Context, id string) (*user.User, error) {
	uid, ok := postgres.ParseID(id)
	if !ok {
		return nil, nil
	}
	return r.getOne(ctx, r.Conn(ctx),
		`SELECT * FROM users WHERE id = $1 /* tenant-scope-exempt: identity load by id */`, uid)
}

// FindScopedByID is the tenant-scoped by-id read behind the admin read-one
// endpoint — FindAll's scoping, not FindByID's.
func (r *Repository) FindScopedByID(ctx context.Context, id string) (*user.User, error) {
	uid, ok := postgres.ParseID(id)
	if !ok {
		return nil, nil
	}
	return r.getOne(ctx, r.Conn(ctx),
		`SELECT * FROM users WHERE id = $1 AND tenant_id = $2 AND role <> 'superadmin'`,
		uid, r.Tenant(ctx))
}

// FindByNickname is a global identity lookup: the login lookup, and the
// uniqueness pre-check every create and rename runs. nickname is unique across
// ALL tenants, so the lookup must see all of them on every plane — it runs on the
// system role (SystemConn), outside a tenant transaction's row-level security.
func (r *Repository) FindByNickname(ctx context.Context, nickname string) (*user.User, error) {
	return r.getOne(ctx, r.SystemConn(ctx),
		`SELECT * FROM users WHERE nickname = $1
		 /* tenant-scope-exempt: global identity lookup (login, nickname uniqueness) */`,
		nickname)
}

func (r *Repository) getOne(
	ctx context.Context,
	c postgres.Conn,
	q string,
	args ...any,
) (*user.User, error) {
	var u user.User
	err := c.GetContext(ctx, &u, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) FindAll(ctx context.Context) ([]user.User, error) {
	var users []user.User
	err := r.Conn(ctx).SelectContext(ctx, &users,
		`SELECT * FROM users WHERE tenant_id = $1 AND role <> 'superadmin'
		  ORDER BY nickname`+database.CollateSort,
		r.Tenant(ctx))
	return users, err
}

// FindByIDAcrossTenants is the platform-plane read-one: one user in ANY tenant,
// joined to its tenant name. Not-found returns (nil, nil).
func (r *Repository) FindByIDAcrossTenants(
	ctx context.Context,
	id string,
) (*user.PlatformRow, error) {
	uid, ok := postgres.ParseID(id)
	if !ok {
		return nil, nil
	}
	var row user.PlatformRow
	err := r.SystemConn(ctx).GetContext(ctx, &row,
		`SELECT u.id, u.nickname, u.email, u.role, u.active, u.tenant_id,
		        t.name AS tenant_name, u.last_login_at
		   FROM users u
		   JOIN tenants t ON t.id = u.tenant_id /* tenant-scope-exempt: platform superadmin */
		  WHERE u.id = $1`, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// CountAcrossTenants counts every user (all tenants, including superadmins) for
// the platform dashboard.
func (r *Repository) CountAcrossTenants(ctx context.Context) (int, error) {
	var n int
	err := r.SystemConn(ctx).GetContext(ctx, &n,
		`SELECT COUNT(*) FROM users /* tenant-scope-exempt: platform superadmin */`)
	return n, err
}

// CountByActive returns the tenant-scoped total + active user counts for the
// admin dashboard in one query — FindAll's scoping.
func (r *Repository) CountByActive(ctx context.Context) (int, int, error) {
	var row struct {
		Total  int `db:"total"`
		Active int `db:"active"`
	}
	err := r.Conn(ctx).GetContext(ctx, &row,
		`SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE active) AS active
		   FROM users WHERE tenant_id = $1 AND role <> 'superadmin'`, r.Tenant(ctx))
	return row.Total, row.Active, err
}

// UpdateAcrossTenants is the platform-plane write — any tenant, never a
// superadmin row, never a tenant move (tenant_id is not in the SET clause).
func (r *Repository) UpdateAcrossTenants(ctx context.Context, u *user.User) error {
	id, ok := postgres.ParseID(u.ID)
	if !ok {
		return requireOneRow(nil, nil)
	}
	res, err := r.SystemConn(ctx).ExecContext(ctx,
		`UPDATE users SET nickname = $1, password_hash = $2, email = $3, role = $4, active = $5, updated_at = $6
		  WHERE id = $7 AND role <> 'superadmin' /* tenant-scope-exempt: platform superadmin */`,
		u.Nickname, u.PasswordHash, u.Email, u.Role, u.Active, u.UpdatedAt, id)
	return requireOneRow(res, nicknameTaken(err))
}

// DeleteAcrossTenants is the platform-plane delete — same scope and floor as
// UpdateAcrossTenants.
func (r *Repository) DeleteAcrossTenants(ctx context.Context, id string) error {
	uid, ok := postgres.ParseID(id)
	if !ok {
		return requireOneRow(nil, nil)
	}
	res, err := r.SystemConn(ctx).ExecContext(ctx,
		`DELETE FROM users WHERE id = $1 AND role <> 'superadmin' /* tenant-scope-exempt: platform superadmin */`,
		uid)
	return requireOneRow(res, err)
}

// RecordLogin stamps last_login_at on successful login. Raw system pool
// (r.DB.System()), the twin of the SQLite raw pool: the stamp auto-commits on its
// own, independent of any transaction in ctx. Best-effort.
func (r *Repository) RecordLogin(ctx context.Context, userID string) error {
	id, ok := postgres.ParseID(userID)
	if !ok {
		return nil
	}
	_, err := r.DB.System().ExecContext(ctx,
		`/* tenant-scope-exempt: stamp successful login by id */
		 UPDATE users SET last_login_at = `+postgres.NowExpr+` WHERE id = $1`,
		id)
	return err
}

// RecordFailedLogin runs the whole counter decision (reset after the window,
// increment otherwise, lock when the threshold is reached) in ONE statement, so
// concurrent failed logins for the same row cannot interleave — the row lock
// serializes them. Raw system pool for the same reason as RecordLogin: a rollback
// of any caller's transaction must not erase the counter, or brute-force
// protection becomes a no-op. The CASE branches read the pre-update values; see
// the SQLite twin for the logic.
func (r *Repository) RecordFailedLogin(
	ctx context.Context,
	userID string,
	threshold int,
	window, lockDuration time.Duration,
) (*time.Time, error) {
	id, ok := postgres.ParseID(userID)
	if !ok {
		return nil, sql.ErrNoRows // no such row, as the statement would report on SQLite
	}
	q := `
		/* tenant-scope-exempt: brute-force counter, runs pre/at-login by user id */
		UPDATE users SET
		    failed_login_attempts = CASE
		        WHEN last_failed_login_at IS NULL
		             OR last_failed_login_at < ` + postgres.NowExpr + ` - make_interval(secs => $1)
		        THEN 1
		        WHEN failed_login_attempts + 1 >= $2
		        THEN 0
		        ELSE failed_login_attempts + 1
		    END,
		    last_failed_login_at = ` + postgres.NowExpr + `,
		    locked_until = CASE
		        WHEN last_failed_login_at IS NOT NULL
		             AND last_failed_login_at >= ` + postgres.NowExpr + ` - make_interval(secs => $1)
		             AND failed_login_attempts + 1 >= $2
		        THEN ` + postgres.NowPlus("$3") + `
		        ELSE locked_until
		    END
		WHERE id = $4
		RETURNING locked_until`

	var locked sql.NullTime
	err := r.DB.System().GetContext(ctx, &locked, q,
		window.Seconds(), threshold, lockDuration.Seconds(), id)
	if err != nil {
		return nil, err
	}
	// Only a lock THIS attempt set is "the new lock" (the port contract): a past
	// locked_until left by an expired lock is not one — see the SQLite twin (F-045).
	if !locked.Valid || !locked.Time.After(time.Now()) {
		return nil, nil
	}
	return &locked.Time, nil
}

// ResetFailedLogin clears the counter on successful login — raw system pool, like
// RecordFailedLogin.
func (r *Repository) ResetFailedLogin(ctx context.Context, userID string) error {
	id, ok := postgres.ParseID(userID)
	if !ok {
		return nil
	}
	_, err := r.DB.System().ExecContext(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = $1
		 /* tenant-scope-exempt: clear brute-force counter on successful login */`,
		id)
	return err
}

// requireOneRow turns a by-id mutation result into a not-found error when it
// matched no row — the twin of the SQLite repository's: a scoped UPDATE/DELETE
// that changes 0 rows (target absent, malformed id, or excluded by the WHERE
// guard) must never pass as a silent success (F-023, F-039). A nil res with a nil
// err is a statement that could not match anything (a malformed id).
func requireOneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	var n int64
	if res != nil {
		if n, err = res.RowsAffected(); err != nil {
			return err
		}
	}
	if n == 0 {
		//gkerrf:exempt requireOneRow guard - by-id mutations surface via redirect/toast, no form field maps id
		return &shared.ValidationError{Field: "id", Key: msgkey.UserNotFound}
	}
	return nil
}
