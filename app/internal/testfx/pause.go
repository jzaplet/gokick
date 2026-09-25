package testfx

import (
	"sync"
	"testing"
	"time"

	"gokick/app/domain/shared"
)

// RaceEdit runs a concurrent write in the middle of an edit — a command that
// loads a row and writes it back — and returns what each returned. edit builds
// the command over the hasher it gets: the first Hash call stops the edit (an
// edit hashes a new password between its read and its write), concurrent then
// runs alongside it, and the edit goes on once concurrent has had time to commit
// — or to wait for the edit, if the edit locked the row it read.
func (f *Fixture) RaceEdit(
	t *testing.T,
	edit func(hasher shared.PasswordHasher) error,
	concurrent func() error,
) (editErr, concurrentErr error) {
	t.Helper()
	hasher := &pausingHasher{
		PasswordHasher: f.Hasher,
		paused:         make(chan struct{}),
		resume:         make(chan struct{}),
	}
	edited := make(chan error, 1)
	go func() { edited <- edit(hasher) }()
	select {
	case <-hasher.paused: // the edit has read the row and is about to write it back
	case err := <-edited:
		t.Fatalf("the edit ended (%v) before it stopped between its read and its write", err)
	}

	raced := make(chan error, 1)
	go func() { raced <- concurrent() }()
	time.Sleep(300 * time.Millisecond) // concurrent commits now, unless it waits
	close(hasher.resume)
	return <-edited, <-raced
}

// pausingHasher stops the first Hash call until resume closes. It returns a fixed
// hash without hashing: nothing reads it back, and real hashing (slow under
// -race) would only keep the concurrent write waiting on the edit's lock.
type pausingHasher struct {
	shared.PasswordHasher
	paused    chan struct{}
	resume    chan struct{}
	pauseOnce sync.Once
}

func (h *pausingHasher) Hash(string) (string, error) {
	h.pauseOnce.Do(func() { close(h.paused) })
	<-h.resume
	return "paused-hash", nil
}
