package audit

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
)

// Repository persists AuditRecord rows. Uses r.DB.DB() (raw pool)
// instead of r.Conn(ctx) so the write commits independently of any
// business transaction sitting on the context — audit must survive
// rollbacks of the work it observes.
type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, rec *shared.AuditRecord) error {
	const q = `INSERT INTO audit_log
	    (id, actor_user_id, actor_ip, action, target_type, target_id, metadata, created_at)
	    VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.DB.DB().ExecContext(ctx, q,
		rec.ID,
		rec.ActorUserID,
		rec.ActorIP,
		rec.Action,
		rec.TargetType,
		rec.TargetID,
		rec.Metadata,
		rec.CreatedAt,
	)
	return err
}
