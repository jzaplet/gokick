package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestParseRateRule_AcceptedShapes(t *testing.T) {
	t.Parallel()
	cases := map[string]RateRule{
		"10/sec":   {Tokens: 10, Per: time.Second},
		"60/min":   {Tokens: 60, Per: time.Minute},
		"100/hour": {Tokens: 100, Per: time.Hour},
		"5/30s":    {Tokens: 5, Per: 30 * time.Second},
		"1/2m":     {Tokens: 1, Per: 2 * time.Minute},
	}
	for spec, want := range cases {
		got, err := ParseRateRule(spec)
		if err != nil {
			t.Errorf("%q: %v", spec, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %+v want %+v", spec, got, want)
		}
	}
}

func TestParseRateRule_EmptyReturnsZero(t *testing.T) {
	t.Parallel()
	got, err := ParseRateRule("")
	if err != nil || got != (RateRule{}) {
		t.Fatalf("empty spec must be zero-value with no error, got %+v / %v", got, err)
	}
}

func TestParseRateRule_RejectsBadInput(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{"10", "abc/min", "10/forever", "-1/min", "0/min", "10/"} {
		if _, err := ParseRateRule(spec); err == nil {
			t.Errorf("%q should fail to parse", spec)
		}
	}
}

func TestRateLimiter_AllowsBurstThenBlocks(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(RateRule{Tokens: 3, Per: time.Second}, remoteIP, silentLogger())
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4", now) {
			t.Fatalf("request %d in burst must be allowed", i+1)
		}
	}
	if l.allow("1.2.3.4", now) {
		t.Fatal("4th request inside the same instant must be blocked")
	}
}

func TestRateLimiter_RefillRestoresCapacity(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(RateRule{Tokens: 2, Per: time.Second}, remoteIP, silentLogger())
	now := time.Now()
	for i := 0; i < 2; i++ {
		if !l.allow("ip", now) {
			t.Fatalf("initial burst request %d must succeed", i+1)
		}
	}
	if l.allow("ip", now) {
		t.Fatal("over-burst must be blocked")
	}
	// One full period later, the bucket is back to full.
	later := now.Add(time.Second)
	if !l.allow("ip", later) {
		t.Fatal("first request after refill must be allowed")
	}
	if !l.allow("ip", later) {
		t.Fatal("second request after refill must be allowed")
	}
}

func TestRateLimiter_PerIPBucketsAreIndependent(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(RateRule{Tokens: 1, Per: time.Minute}, remoteIP, silentLogger())
	now := time.Now()
	if !l.allow("ip-a", now) {
		t.Fatal("ip-a must be allowed first")
	}
	if !l.allow("ip-b", now) {
		t.Fatal("ip-b must not share ip-a's bucket")
	}
	if l.allow("ip-a", now) {
		t.Fatal("ip-a second request must be blocked")
	}
}

func TestRateLimiter_SweepDropsIdleBuckets(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(RateRule{Tokens: 1, Per: time.Minute}, remoteIP, silentLogger())
	t0 := time.Now()
	l.allow("ip", t0)
	if len(l.buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(l.buckets))
	}
	l.Sweep(t0.Add(10*time.Minute), 5*time.Minute)
	if len(l.buckets) != 0 {
		t.Fatalf("idle bucket should be swept, got %d", len(l.buckets))
	}
}

func TestRateLimiter_MiddlewareReturns429AndRetryAfter(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(RateRule{Tokens: 1, Per: 30 * time.Second}, remoteIP, silentLogger())
	mw := l.Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: 200.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: got %d want 200", rec.Code)
	}

	// Second request from same IP: 429 with Retry-After.
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After header")
	}
}

func TestRateLimiter_DisabledRuleIsPassThrough(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(RateRule{}, remoteIP, silentLogger()) // zero rule = disabled
	handler := l.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.1.1.1:80"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("disabled limiter must pass through, got %d on iter %d", rec.Code, i)
		}
	}
}

func TestNewIPExtractor_DefaultIgnoresXRealIP(t *testing.T) {
	t.Parallel()
	extract := NewIPExtractor(false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	if got := extract(req); got != "10.0.0.1" {
		t.Fatalf("default extractor must ignore X-Real-IP, got %q", got)
	}
}

func TestNewIPExtractor_TrustProxyPrefersXRealIP(t *testing.T) {
	t.Parallel()
	extract := NewIPExtractor(true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	if got := extract(req); got != "1.2.3.4" {
		t.Fatalf("trust-proxy extractor must prefer X-Real-IP, got %q", got)
	}
}

func TestNewIPExtractor_TrustProxyFallsBackToRemoteAddr(t *testing.T) {
	t.Parallel()
	extract := NewIPExtractor(true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	// No X-Real-IP header → fall back.
	if got := extract(req); got != "10.0.0.1" {
		t.Fatalf("missing X-Real-IP should fall back to RemoteAddr, got %q", got)
	}
}
