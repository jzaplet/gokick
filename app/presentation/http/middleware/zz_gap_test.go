package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// roadmap-69: the rate-limit IPExtractor is shared with the audit IP middleware,
// so flipping APP_TRUST_PROXY_HEADERS flips the bucketing key for BOTH. The
// extractor's own trust-proxy logic is pinned in ratelimit_test.go
// (TestNewIPExtractor_*) and the audit/IP side in zz_audit_test.go
// (TestIPMiddleware_TrustProxyStashesXRealIP). What no existing test pins is
// that RateLimiter.Middleware actually KEYS its buckets on the injected
// extractor's output rather than hard-coding RemoteAddr — every RateLimiter
// test feeds remoteIP directly. Without this test the mutation at
// ratelimit.go:121 (l.extract(r) -> remoteIP(r)) survives: behind a trusted
// proxy the limiter would collapse every client onto the proxy's RemoteAddr
// (mass lockout) and APP_TRUST_PROXY_HEADERS would silently not apply to rate
// limiting.
//
// The trust-proxy=true case is the discriminating one: only here does the
// extractor output (X-Real-IP) diverge from remoteIP(RemoteAddr), so a mutation
// that ignored the extractor would behave identically under false but visibly
// wrong under true.
func TestRateLimiter_MiddlewareKeysBucketsOnInjectedExtractor(t *testing.T) {
	t.Parallel()

	// One token per IP, trust-proxy extractor → buckets are keyed on X-Real-IP.
	l := NewRateLimiter(RateRule{Tokens: 1, Per: time.Minute}, NewIPExtractor(true), silentLogger())
	handler := l.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	serve := func(realIP, remoteAddr string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
		req.Header.Set("X-Real-IP", realIP)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// First request from logical client A (X-Real-IP=A) consumes A's token.
	if code := serve("198.51.100.1", "10.0.0.1:1111"); code != http.StatusOK {
		t.Fatalf("first request for X-Real-IP A: got %d want 200", code)
	}

	// Second request with the SAME X-Real-IP but a DIFFERENT RemoteAddr must
	// hit the SAME bucket → 429. If the limiter keyed on RemoteAddr instead of
	// the injected extractor, this would be a fresh bucket and wrongly 200.
	if code := serve("198.51.100.1", "10.0.0.2:2222"); code != http.StatusTooManyRequests {
		t.Fatalf("same X-Real-IP, different RemoteAddr: got %d want 429 "+
			"(limiter must key on the injected extractor, not RemoteAddr)", code)
	}

	// A different logical client B (X-Real-IP=B) has its own bucket → 200,
	// proving the key tracks the extractor output rather than rejecting globally.
	if code := serve("198.51.100.2", "10.0.0.1:1111"); code != http.StatusOK {
		t.Fatalf("distinct X-Real-IP B: got %d want 200 (independent bucket)", code)
	}
}
