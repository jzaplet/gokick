package shared

import "context"

type Transactor interface {
	BeginTx(ctx context.Context) (context.Context, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type noTxKey struct{}

// ContextForbidTx marks ctx as a zone where opening a DB transaction is forbidden.
// The durable run worker marks the handler ctx with it: a long-running run handler
// inside a transaction would hold the global SQLite write lock for the run's whole
// lifetime (minutes to hours) and freeze every other write — the exact failure the
// outside-transaction model exists to avoid. The Transactor's BeginTx honors it and
// fails closed, so an accidental tx in a run handler surfaces immediately instead of
// silently freezing the database. Run handlers persist state via the Checkpointer;
// for transactional side-work they enqueue a command/job that runs in its own short
// transaction outside the run.
func ContextForbidTx(ctx context.Context) context.Context {
	return context.WithValue(ctx, noTxKey{}, true)
}

// IsTxForbidden reports whether ctx is a no-transaction zone (see ContextForbidTx).
func IsTxForbidden(ctx context.Context) bool {
	forbidden, _ := ctx.Value(noTxKey{}).(bool)
	return forbidden
}
