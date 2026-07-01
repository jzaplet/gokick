package worker

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// silentLogger discards output — worker tests assert on state/DB, not log lines.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// backoff() is unexported; this file is package worker so it is callable directly.
// We assert literal durations (5s/10s/20s/1h), never defaultBaseBackoff — comparing a
// value against the very constant it is built from is a tautology. The literals pin
// the documented schedule the run worker uses for retry/park rescheduling.
func TestBackoff_ExponentialScheduleAndCap(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 5 * time.Second},  // first failure → base 5s
		{2, 10 * time.Second}, // 2^1 * 5s
		{3, 20 * time.Second}, // 2^2 * 5s
		{4, 40 * time.Second}, // 2^3 * 5s
		{11, time.Hour},       // 2^10 * 5s = 5120s > 1h → capped
		{50, time.Hour},       // far past the cap stays capped
		{0, 5 * time.Second},  // attempts<1 clamp → base
		{-1, 5 * time.Second}, // negative clamp → base (not the overflow cap)
	}
	for _, c := range cases {
		if got := backoff(c.attempts); got != c.want {
			t.Fatalf("backoff(%d): got %v want %v", c.attempts, got, c.want)
		}
	}
}
