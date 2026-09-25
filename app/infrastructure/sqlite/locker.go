//go:build !nosqlite

package sqlite

import "context"

// Locker is shared.Locker on SQLite: every lock is granted. A SQLite database is
// a file on one host, served by one `serve` process, so there is no other replica
// to exclude. (Two `serve` processes on one file would each hold every lock and
// each run every scheduler job.)
type Locker struct{}

func (Locker) Hold(context.Context, string) (bool, error) { return true, nil }

func (Locker) Release(context.Context, string) error { return nil }
