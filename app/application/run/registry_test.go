package run

import (
	"context"
	"testing"
	"time"

	"gokick/app/domain/run"
)

func noopHandler(context.Context, *run.Run, Checkpointer) error { return nil }

// FireAndForget is the "job" shape (default lease, no checkpoint expected); Durable
// is the "run" shape (explicit lease). Both carry an optional per-attempt timeout.
func TestRegistrationHelpers(t *testing.T) {
	ff := FireAndForget(noopHandler, 30*time.Second)
	if ff.Lease != 0 || ff.Timeout != 30*time.Second {
		t.Fatalf("FireAndForget: lease=%v timeout=%v, want 0 / 30s", ff.Lease, ff.Timeout)
	}
	d := Durable(noopHandler, 5*time.Minute, time.Hour)
	if d.Lease != 5*time.Minute || d.Timeout != time.Hour {
		t.Fatalf("Durable: lease=%v timeout=%v, want 5m / 1h", d.Lease, d.Timeout)
	}
}

// Lookup returns the effective lease (kind's or default), the timeout, and known.
func TestLookup_LeaseTimeoutAndDefault(t *testing.T) {
	reg, err := NewHandlerRegistry(map[string]Registration{
		"durable":    Durable(noopHandler, 5*time.Minute, time.Hour),
		"fireforget": FireAndForget(noopHandler, 0), // no lease, no timeout
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	_, lease, timeout, known := reg.Lookup("durable")
	if !known || lease != 5*time.Minute || timeout != time.Hour {
		t.Fatalf("durable: known=%v lease=%v timeout=%v, want true/5m/1h", known, lease, timeout)
	}

	_, lease, timeout, known = reg.Lookup("fireforget")
	if !known || lease != 2*time.Second || timeout != 0 {
		t.Fatalf(
			"fireforget: known=%v lease=%v timeout=%v, want true/2s(default)/0",
			known,
			lease,
			timeout,
		)
	}

	if _, _, _, known = reg.Lookup("nope"); known {
		t.Fatal("unknown kind must report known=false")
	}
}

// Timeout gets the same plausibility scrutiny as Lease: a negative value (would
// silently disable the timeout) and a positive sub-ms value (would instant-expire
// every attempt) are units typos, rejected loudly at boot.
func TestNewHandlerRegistry_RejectsImplausibleTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeout time.Duration
	}{
		{"negative", -time.Second},
		{"positive sub-ms", time.Microsecond},
	} {
		_, err := NewHandlerRegistry(map[string]Registration{
			"k": {Handler: noopHandler, Timeout: tc.timeout},
		}, time.Second)
		if err == nil {
			t.Fatalf("%s timeout (%s) must be rejected at registry build", tc.name, tc.timeout)
		}
	}
}
