package run

import (
	"errors"
	"testing"
	"time"
)

// F-100: the terminal/lease-state derivation has one domain source (Status /
// IsTerminal), mirroring the SQL sqlite.NotTerminalClause. The read path calls it
// instead of re-expressing the switch.
func TestRun_StatusAndIsTerminal(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	ptr := func(t time.Time) *time.Time { return &t }
	future, past := ptr(now.Add(time.Hour)), ptr(now.Add(-time.Hour))

	cases := []struct {
		name     string
		run      Run
		want     string
		terminal bool
	}{
		{"completed", Run{CompletedAt: past}, "completed", true},
		{"failed", Run{FailedAt: past}, "failed", true},
		{"cancelled", Run{CancelledAt: past}, "cancelled", true},
		{"running (live lease)", Run{LockedUntil: future}, "running", false},
		{"pending (no lease)", Run{}, "pending", false},
		{"pending (expired lease)", Run{LockedUntil: past}, "pending", false},
		// Terminal wins over a live lease (a completed run that still shows a lease).
		{"completed beats lease", Run{CompletedAt: past, LockedUntil: future}, "completed", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.run.Status(now); got != tc.want {
				t.Errorf("Status = %q, want %q", got, tc.want)
			}
			if got := tc.run.IsTerminal(); got != tc.terminal {
				t.Errorf("IsTerminal = %v, want %v", got, tc.terminal)
			}
		})
	}
}

// NewRun owns the construction invariants so no enqueue path (dispatcher,
// debug_run, or any future caller) can persist a Run that violates them —
// F-021. Before, the invariant was only enforced by the dispatcher and the
// type's doc merely claimed it.
func TestNewRun_RejectsInvalidArgs(t *testing.T) {
	t.Run("empty kind", func(t *testing.T) {
		r, err := NewRun("", []byte(`{}`), 0)
		if !errors.Is(err, ErrEmptyKind) {
			t.Fatalf("want ErrEmptyKind, got %v", err)
		}
		if r != nil {
			t.Fatalf("want nil Run on error, got %+v", r)
		}
	})

	t.Run("negative maxRetries", func(t *testing.T) {
		for _, n := range []int{-1, -100} {
			r, err := NewRun("agent", []byte(`{}`), n)
			if !errors.Is(err, ErrNegativeRetries) {
				t.Fatalf("maxRetries=%d: want ErrNegativeRetries, got %v", n, err)
			}
			if r != nil {
				t.Fatalf("maxRetries=%d: want nil Run on error, got %+v", n, r)
			}
		}
	})
}

// A valid construction yields a pending Run with the invariant-bearing fields set
// and no error — maxRetries=0 (one attempt, no retry) is the boundary that must pass.
func TestNewRun_ValidArgs(t *testing.T) {
	r, err := NewRun("agent", []byte(`{"input":"x"}`), 0)
	if err != nil {
		t.Fatalf("valid args must not error, got %v", err)
	}
	if r.ID == "" {
		t.Fatal("want a generated id")
	}
	if r.Kind != "agent" {
		t.Fatalf("kind = %q, want agent", r.Kind)
	}
	if r.MaxRetries != 0 {
		t.Fatalf("maxRetries = %d, want 0", r.MaxRetries)
	}
}
