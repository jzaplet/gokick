package shared

import (
	"sync"
	"testing"
)

func TestAuditCollector_RecordAndDrain(t *testing.T) {
	t.Parallel()
	c := &AuditCollector{}
	c.Record(AuditEvent{Action: "a"})
	c.Record(AuditEvent{Action: "b"})

	got := c.Drain()
	if len(got) != 2 || got[0].Action != "a" || got[1].Action != "b" {
		t.Fatalf("drain order: %+v", got)
	}
	if rest := c.Drain(); len(rest) != 0 {
		t.Fatalf("collector should be empty after drain, got %d", len(rest))
	}
}

// Audit collectors must tolerate goroutines spawned by the handler — the
// mutex covers the slice append. The test fans out 50 writers; if append
// raced, -race would fail it.
func TestAuditCollector_ConcurrentRecord(t *testing.T) {
	t.Parallel()
	c := &AuditCollector{}
	const writers, perWriter = 50, 20
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				c.Record(AuditEvent{Action: "concurrent"})
			}
		}()
	}
	wg.Wait()
	if got := len(c.Drain()); got != writers*perWriter {
		t.Fatalf("expected %d events, got %d", writers*perWriter, got)
	}
}

func TestAuditCollectorFromContext_ReturnsThrowawayOutsideBus(t *testing.T) {
	t.Parallel()
	c := AuditCollectorFromContext(t.Context())
	if c == nil {
		t.Fatal("must never return nil")
	}
	// Throwaway should still accept Record (no panic) — handlers shouldn't
	// have to nil-check just because they're being tested directly.
	c.Record(AuditEvent{Action: "x"})
}
