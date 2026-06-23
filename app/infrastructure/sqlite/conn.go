package sqlite

import (
	"context"
	"database/sql"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"
)

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
func (b *BaseRepository) Tenant(ctx context.Context) string {
	if id := shared.TenantIDFromContext(ctx); id != "" {
		return id
	}
	if b.DB.Multitenant() {
		panic("sqlite: tenant required but absent from context (APP_MULTITENANCY=true)")
	}
	return shared.DefaultTenantID
}
