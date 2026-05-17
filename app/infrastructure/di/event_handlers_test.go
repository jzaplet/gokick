package di

import (
	"testing"
)

// Catches a "someone added an entry with empty event name or nil handler"
// regression at test time. EventBus.Register would silently accept either,
// so the production registry deserves its own sanity gate.
func TestProvideEventHandlers_ValidEntries(t *testing.T) {
	t.Parallel()

	for i, e := range provideEventHandlers() {
		if e.Event == "" {
			t.Errorf("entry %d: empty Event name", i)
		}
		if e.Handler == nil {
			t.Errorf("entry %d (event=%q): nil Handler", i, e.Event)
		}
	}
}
