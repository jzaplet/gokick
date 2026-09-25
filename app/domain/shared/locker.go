package shared

import "context"

// Locker hands out named locks that exclude every other process on the same
// database — the other replicas of `serve` — so work that must run in one place
// at a time (a scheduler job) runs in one. A lock belongs to the Locker that took
// it and stays with it until Release, or until the process loses its database
// session (it exits, crashes or is cut off); then another Locker can take it.
//
// On a database only one process serves (SQLite) there is nobody to exclude, and
// every lock is granted.
type Locker interface {
	// Hold takes the lock named key, or confirms this Locker still holds it, and
	// reports whether it does. It never waits: a lock another process holds is
	// reported as false. Call it again before each piece of guarded work — a
	// lock held a minute ago may since have been lost with the session.
	Hold(ctx context.Context, key string) (bool, error)
	// Release gives up the lock named key; releasing a lock not held is a no-op.
	Release(ctx context.Context, key string) error
}
