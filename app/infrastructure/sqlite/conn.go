package sqlite

import (
	"context"
	"database/sql"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"
)

// MsPrecisionUTC normalizes a Go time.Time to UTC + millisecond precision before
// it crosses into SQLite. ncruces' WASM SQLite 'now' ticks at ~1 ms and trails
// Go's time.Now() by up to ~1 ms, so a time written at µs precision can lose the
// julianday(col) <= julianday('now') race and a freshly-enqueued row be missed.
// Truncating to ms removes it; reads round-trip the exact same ms value. Shared by
// every repo that writes Go-sourced times (job, run) so the time discipline cannot
// drift between queues — see /gk-repositories.
func MsPrecisionUTC(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}

// Conn is the common interface satisfied by both *sqlx.DB and *sqlx.Tx.
type Conn interface {
	NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
}

// BaseRepository provides transaction-aware DB connection resolution.
// Embed it in concrete repositories to avoid repeating ConnFromContext calls.
type BaseRepository struct {
	DB *database.SqliteManager
}

func (b *BaseRepository) Conn(ctx context.Context) Conn {
	if tx := database.TxFromContext(ctx); tx != nil {
		return tx
	}
	return b.DB.DB()
}

// Tenant returns the tenant id to scope a query by — read from the value
// TenantMiddleware put in ctx. When none is present it falls back to the default
// tenant in single-tenant mode, or panics in multitenant mode
// (APP_MULTITENANCY=true): there, a missing tenant is a bug that must NOT
// silently scope to the default tenant (a cross-tenant leak).
//
// It delegates the "non-empty ? keep : multitenant ? fail : default" decision to
// shared.RequireTenant (the write side's resolver) so the read and write sides
// share ONE classification and can't drift. The only difference is the reaction to
// a missing tenant under multitenancy: a read reaching here without a tenant is a
// middleware bug, so the error becomes a panic rather than a value every caller
// must thread through.
func (b *BaseRepository) Tenant(ctx context.Context) string {
	id, err := shared.RequireTenant(shared.TenantIDFromContext(ctx), b.Multitenancy())
	if err != nil {
		panic("sqlite: tenant required but absent from context (APP_MULTITENANCY=true)")
	}
	return id
}

// Multitenancy reports the configured enforcement mode as the Wire-distinct shared
// type, for the repo write guards (shared.RequireTenant / shared.AssertTenantScope) —
// so they don't each restate shared.Multitenancy(r.DB.Multitenant()).
func (b *BaseRepository) Multitenancy() shared.Multitenancy {
	return shared.Multitenancy(b.DB.Multitenant())
}
