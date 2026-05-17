package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gokick/app/presentation/http/response"
)

// RateRule is "Tokens requests per Per duration" — a token bucket spec.
// Parsed from configuration via ParseRateRule.
type RateRule struct {
	Tokens int
	Per    time.Duration
}

// ParseRateRule accepts "N/sec", "N/min", "N/hour", "N/Xs", "N/Xm", "N/Xh".
// Empty input returns the zero RateRule and no error so callers can use it
// as the "disabled" sentinel.
func ParseRateRule(spec string) (RateRule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return RateRule{}, nil
	}
	parts := strings.SplitN(spec, "/", 2)
	if len(parts) != 2 {
		return RateRule{}, fmt.Errorf("rate rule %q: expected N/duration", spec)
	}
	tokens, err := strconv.Atoi(parts[0])
	if err != nil || tokens <= 0 {
		return RateRule{}, fmt.Errorf("rate rule %q: tokens must be positive integer", spec)
	}
	per, err := parseDuration(parts[1])
	if err != nil {
		return RateRule{}, fmt.Errorf("rate rule %q: %w", spec, err)
	}
	return RateRule{Tokens: tokens, Per: per}, nil
}

func parseDuration(s string) (time.Duration, error) {
	switch s {
	case "sec", "second":
		return time.Second, nil
	case "min", "minute":
		return time.Minute, nil
	case "hour":
		return time.Hour, nil
	}
	return time.ParseDuration(s)
}

// IPExtractor returns the client IP. NewIPExtractor wires it to either
// RemoteAddr (safe default) or to X-Real-IP (opt-in for trusted proxies).
type IPExtractor func(*http.Request) string

func NewIPExtractor(trustProxy bool) IPExtractor {
	if trustProxy {
		return func(r *http.Request) string {
			if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
				return ip
			}
			return remoteIP(r)
		}
	}
	return remoteIP
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimiter is a per-IP token bucket. Buckets accumulate idle and are
// swept by a background janitor (Run) to keep memory bounded.
type RateLimiter struct {
	rule    RateRule
	extract IPExtractor
	logger  *slog.Logger

	refillPerSec float64

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens    float64
	updatedAt time.Time
}

func NewRateLimiter(rule RateRule, extract IPExtractor, logger *slog.Logger) *RateLimiter {
	return &RateLimiter{
		rule:         rule,
		extract:      extract,
		logger:       logger,
		refillPerSec: float64(rule.Tokens) / rule.Per.Seconds(),
		buckets:      map[string]*bucket{},
	}
}

// Middleware enforces the rate on incoming requests. Returns 429 with a
// Retry-After header when the bucket is empty. If the underlying rule is
// the zero value (disabled), the middleware is a pass-through.
func (l *RateLimiter) Middleware() func(http.Handler) http.Handler {
	if l.rule.Tokens == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(l.extract(r), time.Now()) {
				w.Header().Set("Retry-After", strconv.Itoa(int(l.rule.Per.Seconds())))
				response.Error(w, http.StatusTooManyRequests, errors.New("too many requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (l *RateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		// First request from this IP: hand out the bucket already
		// minus the token we're about to consume.
		l.buckets[key] = &bucket{tokens: float64(l.rule.Tokens) - 1, updatedAt: now}
		return true
	}

	elapsed := now.Sub(b.updatedAt).Seconds()
	b.tokens += elapsed * l.refillPerSec
	if b.tokens > float64(l.rule.Tokens) {
		b.tokens = float64(l.rule.Tokens)
	}
	b.updatedAt = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Sweep drops buckets idle for at least dropAfter so a long-running
// process doesn't grow the map unbounded under stuffing attacks.
func (l *RateLimiter) Sweep(now time.Time, dropAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if now.Sub(b.updatedAt) > dropAfter {
			delete(l.buckets, k)
		}
	}
}

// Run sweeps idle buckets at interval until ctx is cancelled. Spawn it
// once per limiter from Server.Start.
func (l *RateLimiter) Run(ctx context.Context, interval, dropAfter time.Duration) {
	if l.rule.Tokens == 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			l.Sweep(t, dropAfter)
		}
	}
}
