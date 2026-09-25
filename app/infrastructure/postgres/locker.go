package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
)

// lockNamespace is the first key of every lock the Locker takes (the two-int
// advisory lock form: namespace, hashtext(key)). It keeps the Locker's locks apart
// from the single-bigint lock goose takes around the migrations.
const lockNamespace = 7437261

// Locker is shared.Locker on Postgres session advisory locks
// (pg_try_advisory_lock). A session lock belongs to a database session, not a
// transaction, so all the locks of one Locker live on one dedicated connection of
// the system pool: taken on the first lock, returned to the pool once the last one
// is released — never while it still holds a lock, or the next borrower would
// inherit it. When the session ends (the process exits or crashes, the
// connection breaks) Postgres drops its locks and another replica can take them.
type Locker struct {
	db *Manager

	mu   sync.Mutex
	conn *sql.Conn
	held map[string]bool
}

// NewLocker returns the Locker of one process.
func NewLocker(db *Manager) *Locker {
	return &Locker{db: db, held: map[string]bool{}}
}

// Hold takes the lock named key or confirms it is still held. A held lock is
// checked against pg_locks on its session every time: if the session is gone,
// so are its locks, and Hold starts over on a fresh connection — where another
// replica may hold the lock by now.
func (l *Locker) Hold(ctx context.Context, key string) (bool, error) {
	// An ended ctx must not reach the session: pgx reports a statement on it as a
	// bad connection, database/sql then closes the connection, and every lock on
	// it — some perhaps guarding a job that runs right now — would go with it.
	if err := ctx.Err(); err != nil {
		return false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.held[key] {
		var still bool
		err := l.conn.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_locks
			 WHERE locktype = 'advisory' AND pid = pg_backend_pid() AND granted
			   AND classid = $1 AND objid = hashtext($2)::oid AND objsubid = 2)`,
			lockNamespace, key).Scan(&still)
		if err == nil && still {
			return true, nil
		}
		// The session, or the lock on it, is gone: start over.
		l.discard()
	}

	if l.conn == nil {
		conn, err := l.db.system.Conn(ctx)
		if err != nil {
			return false, err
		}
		l.conn = conn
	}
	var got bool
	if err := l.conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1, hashtext($2))`, lockNamespace, key).Scan(&got); err != nil {
		l.discard()
		return false, err
	}
	if got {
		l.held[key] = true
		return true, nil
	}
	return false, l.returnIfIdle(ctx)
}

// Release gives up the lock named key. If unlocking fails, the connection is
// discarded instead: its session ends, and the lock with it.
func (l *Locker) Release(ctx context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.held[key] {
		return nil
	}
	delete(l.held, key)
	if _, err := l.conn.ExecContext(ctx,
		`SELECT pg_advisory_unlock($1, hashtext($2))`, lockNamespace, key); err != nil {
		l.discard()
		return err
	}
	return l.returnIfIdle(ctx)
}

// returnIfIdle hands the connection back to the pool once it holds no lock,
// unlocking everything on it first so no lock can ride into the pool.
func (l *Locker) returnIfIdle(ctx context.Context) error {
	if l.conn == nil || len(l.held) > 0 {
		return nil
	}
	if _, err := l.conn.ExecContext(ctx, `SELECT pg_advisory_unlock_all()`); err != nil {
		l.discard()
		return err
	}
	err := l.conn.Close()
	l.conn = nil
	return err
}

// discard closes the connection without returning it to the pool — its session
// ends and takes every lock on it along — and forgets those locks.
func (l *Locker) discard() {
	if l.conn != nil {
		// Reporting ErrBadConn from Raw makes database/sql close the connection
		// rather than pool it.
		_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = l.conn.Close()
		l.conn = nil
	}
	clear(l.held)
}
