package worker

import (
	"math"
	"time"
)

// defaultBaseBackoff is the base unit of the exponential retry/park backoff used by
// the run worker.
const defaultBaseBackoff = 5 * time.Second

// Log-attribute keys shared within the worker package (snake_case; sloglint
// no-raw-keys). Run-worker-specific keys live in run_worker.go.
const (
	logKeyKinds = "kinds"
	logKeyPanic = "panic"
	logKeyStack = "stack"
)

// backoff returns 2^(attempts-1) * defaultBaseBackoff, capped at 1h. attempts is
// 1-based: attempts=1 → wait base before the next attempt.
func backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	exp := math.Pow(2, float64(attempts-1))
	d := time.Duration(exp) * defaultBaseBackoff
	const cap = time.Hour
	if d > cap || d < 0 {
		return cap
	}
	return d
}
