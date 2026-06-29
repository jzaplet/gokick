package shared

import (
	"context"
	"errors"
)

type Transactor interface {
	BeginTx(ctx context.Context) (context.Context, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type noTxKey struct{}

// ContextForbidTx marks ctx as a zone where opening a DB transaction IMPLICITLY is
// forbidden. The durable run worker marks the handler ctx with it: a handler that
// accidentally triggers a transaction (e.g. through a repo/command call) would hold
// the global SQLite write lock for as long as the surrounding work runs and freeze
// every other write. BeginTx honors the marker and fails closed, so an accidental tx
// surfaces immediately instead of silently freezing the database.
//
// Deliberate SHORT transactions are still allowed — and encouraged — for atomic
// multi-row writes: use shared.WithTx, which opens a brief tx the developer scopes
// themselves (write a few rows, commit, continue, maybe another later). The footgun
// is not "a transaction" but "a transaction held open across slow/external I/O (mail,
// an API call)"; keep WithTx blocks short and do slow work outside them — exactly the
// rule that already applies inside a command handler.
func ContextForbidTx(ctx context.Context) context.Context {
	return context.WithValue(ctx, noTxKey{}, true)
}

// IsTxForbidden reports whether ctx is a no-transaction zone (see ContextForbidTx).
func IsTxForbidden(ctx context.Context) bool {
	forbidden, _ := ctx.Value(noTxKey{}).(bool)
	return forbidden
}

// contextAllowTx clears the forbid marker for the scope of one WithTx call, so the
// blessed short-transaction path can open a tx even inside a ContextForbidTx zone,
// while a raw/accidental BeginTx on the surrounding ctx still fails closed.
func contextAllowTx(ctx context.Context) context.Context {
	return context.WithValue(ctx, noTxKey{}, false)
}

type transactorKey struct{}

// ContextWithTransactor injects the Transactor the run worker hands to a handler so
// it can run shared.WithTx. The worker bypasses the bus (where the transaction is
// normally driven by middleware), so it provides the capability on the handler ctx.
func ContextWithTransactor(ctx context.Context, tr Transactor) context.Context {
	return context.WithValue(ctx, transactorKey{}, tr)
}

func transactorFromContext(ctx context.Context) Transactor {
	tr, _ := ctx.Value(transactorKey{}).(Transactor)
	return tr
}

// ErrTxUnavailable is returned by WithTx when no Transactor was injected — it fails
// loud rather than silently skipping the atomicity the caller asked for.
var ErrTxUnavailable = errors.New(
	"shared: WithTx requires a Transactor in context (only available inside a run handler)",
)

// WithTx runs fn inside a single SHORT database transaction — the developer-controlled
// way for a run handler to make several writes atomically (all-or-nothing). Repos
// called with the ctx fn receives join the transaction (via Conn(ctx)); fn returning
// nil commits, a non-nil error or a panic rolls back. Keep fn short and free of
// slow/external I/O — it holds the global SQLite write lock until it returns.
//
// It is available wherever a Transactor was injected (the run worker injects it for
// handlers). Do NOT nest WithTx calls.
func WithTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	tr := transactorFromContext(ctx)
	if tr == nil {
		return ErrTxUnavailable
	}
	txCtx, err := tr.BeginTx(contextAllowTx(ctx))
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tr.Rollback(txCtx)
		}
	}()
	if err = fn(txCtx); err != nil {
		return err
	}
	if err = tr.Commit(txCtx); err != nil {
		return err
	}
	committed = true
	return nil
}
