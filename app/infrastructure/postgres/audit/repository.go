// Package audit implements shared.AuditLogger on Postgres — the twin of the SQLite
// repository.
package audit

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/postgres"
)

// Repository persists AuditRecord rows through the raw system pool
// (r.DB.System()), never r.Conn(ctx): the write commits on its own, independent of
// any business transaction on the context — audit must survive rollbacks of the
// work it observes. The system role may only append to audit_log and read it
// back (see the init migration), so the log is append-only because the database
// says so.
type Repository struct {
	postgres.BaseRepository
}

func NewRepository(db *postgres.Manager) *Repository {
	return &Repository{BaseRepository: postgres.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, rec *shared.AuditRecord) error {
	// metadata is jsonb, which has no empty value: no metadata is NULL.
	var metadata []byte
	if len(rec.Metadata) > 0 {
		metadata = rec.Metadata
	}
	_, err := r.DB.System().ExecContext(ctx,
		`INSERT INTO audit_log
		    (id, actor_user_id, actor_ip, action, target_type, target_id, metadata, created_at)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		rec.ID,
		rec.ActorUserID,
		rec.ActorIP,
		rec.Action,
		rec.TargetType,
		rec.TargetID,
		metadata,
		rec.CreatedAt,
	)
	return err
}
