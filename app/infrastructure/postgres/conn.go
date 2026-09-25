package postgres

import (
	"context"
	"database/sql"
	"errors"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// Conn is the common interface satisfied by *sqlx.DB, *sqlx.Tx and the
// per-statement tenant transaction BaseRepository.Conn falls back to.
type Conn interface {
	NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
}

// BaseRepository resolves where a repository statement runs. Embed it in the
// Postgres repositories, the twin of sqlite.BaseRepository.
type BaseRepository struct {
	DB *Manager
}

// Conn is where a statement runs on behalf of ctx:
//   - the transaction in ctx, when there is one — a bus command or query, whose
//     transaction the Manager already opened on the right role and scope;
//   - on a cross-tenant plane (platform, system), the system pool;
//   - on the tenant plane, a transaction of the statement's own, on the tenant
//     role and scoped to ctx's tenant. Row-level security reads a
//     transaction-local setting, so a tenant-plane statement outside a transaction
//     would see no row at all. This is the path of the work that runs outside the
//     bus — a run handler, an event handler — and it costs a BEGIN and a COMMIT
//     per statement. A tenant plane without a tenant fails closed under
//     multitenancy, like BeginTx.
func (b *BaseRepository) Conn(ctx context.Context) Conn {
	if tx := database.TxFromContext(ctx); tx != nil {
		return tx
	}
	s, err := b.DB.scope(ctx)
	if err != nil {
		return errConn{err}
	}
	if s.crossTenant {
		return b.DB.system
	}
	return scopedConn{m: b.DB, s: s}
}

// SystemConn is where cross-tenant work runs whatever plane ctx is on: the
// platform ports' *AcrossTenants methods (their contract is to reach every tenant,
// as on SQLite; only application/platform calls them, which a gate enforces), the
// worker's claim and lease bookkeeping, the expired-token sweep, the global
// nickname lookup. It joins the transaction in ctx only when that transaction is
// a cross-tenant one; otherwise it runs on the system pool — outside a tenant
// transaction, never inside it. Use it for reads, and for writes that never run
// inside a tenant transaction: a write here does not ride that transaction's
// commit or rollback.
//
// The raw pool (r.DB.System()) is a different thing: a write that must commit on
// its own even inside a transaction (the login counters, the audit log — the
// raw-pool writes of the SQLite adapter).
func (b *BaseRepository) SystemConn(ctx context.Context) Conn {
	if tx := database.TxFromContext(ctx); tx != nil {
		if s, _ := ctx.Value(txScopeKey{}).(txScope); s.crossTenant {
			return tx
		}
	}
	return b.DB.system
}

// Tenant returns the tenant id to scope a query by — the twin of
// sqlite.BaseRepository.Tenant (the same shared.RequireTenant classification):
// the tenant in ctx, else the default tenant in single-tenant mode, else a panic.
// Row-level security scopes the tenant plane anyway; the explicit WHERE tenant_id
// stays as defense in depth, for the planner's index, and because the platform
// and system planes bypass the policies.
func (b *BaseRepository) Tenant(ctx context.Context) string {
	id, err := shared.RequireTenant(shared.TenantIDFromContext(ctx), b.Multitenancy())
	if err != nil {
		panic("postgres: tenant required but absent from context (APP_MULTITENANCY=true)")
	}
	return id
}

// Multitenancy reports the configured enforcement mode as the Wire-distinct shared
// type, for the repo write guards (shared.RequireTenant / shared.AssertTenantScope).
func (b *BaseRepository) Multitenancy() shared.Multitenancy {
	return shared.Multitenancy(b.DB.Multitenant())
}

// scopedConn runs each statement in a transaction of its own on the tenant role,
// scoped to one tenant — see BaseRepository.Conn.
type scopedConn struct {
	m *Manager
	s txScope
}

func (c scopedConn) run(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := c.m.begin(ctx, c.s, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (c scopedConn) NamedExecContext(
	ctx context.Context,
	query string,
	arg any,
) (sql.Result, error) {
	var res sql.Result
	err := c.run(ctx, func(tx *sqlx.Tx) (err error) {
		res, err = tx.NamedExecContext(ctx, query, arg)
		return err
	})
	return res, err
}

func (c scopedConn) ExecContext(
	ctx context.Context,
	query string,
	args ...any,
) (sql.Result, error) {
	var res sql.Result
	err := c.run(ctx, func(tx *sqlx.Tx) (err error) {
		res, err = tx.ExecContext(ctx, query, args...)
		return err
	})
	return res, err
}

func (c scopedConn) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	return c.run(ctx, func(tx *sqlx.Tx) error { return tx.GetContext(ctx, dest, query, args...) })
}

func (c scopedConn) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	return c.run(
		ctx,
		func(tx *sqlx.Tx) error { return tx.SelectContext(ctx, dest, query, args...) },
	)
}

// errConn fails every statement with the error that kept Conn from resolving a
// scope — a tenant plane without a tenant under multitenancy.
type errConn struct{ err error }

func (c errConn) NamedExecContext(context.Context, string, any) (sql.Result, error) {
	return nil, c.err
}

func (c errConn) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, c.err
}

func (c errConn) GetContext(context.Context, any, string, ...any) error { return c.err }

func (c errConn) SelectContext(context.Context, any, string, ...any) error { return c.err }

// ParseID returns id in canonical form, or ok=false when it is no UUID at all.
// Every id column is uuid, and Postgres refuses a malformed literal with an error
// (22P02) where SQLite, comparing text, simply matches no row. A repository
// therefore parses an id from the outside first and treats a malformed one as
// the row that is not there: a lookup returns not-found, a by-id write matches
// nothing.
func ParseID(id string) (string, bool) {
	u, err := uuid.Parse(id)
	if err != nil {
		return "", false
	}
	return u.String(), true
}

// ParseIDs keeps the ids of ids that are UUIDs, in canonical form — a malformed id
// matches no row (see ParseID).
func ParseIDs(ids []string) []string {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if v, ok := ParseID(id); ok {
			valid = append(valid, v)
		}
	}
	return valid
}

// SQLSTATE codes the repositories react to.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
)

// IsUniqueViolation reports whether err is a unique violation of constraint (the
// index or constraint name Postgres reports).
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == codeUniqueViolation && pgErr.ConstraintName == constraint
}

// IsForeignKeyViolation reports whether err is a foreign-key violation.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeForeignKeyViolation
}
