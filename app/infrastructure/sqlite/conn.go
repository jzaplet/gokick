package sqlite

import (
	"context"
	"database/sql"

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
