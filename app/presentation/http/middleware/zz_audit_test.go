package middleware

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"gokick/app/domain/shared"
)

// --- presentation-17: SecurityHeadersMiddleware emits the security baseline,
// with HSTS gated on the hstsEnabled flag. ---

func TestSecurityHeadersMiddleware_EmitsUnconditionalHeaders(t *testing.T) {
	t.Parallel()

	for _, hsts := range []bool{true, false} {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		handler := SecurityHeadersMiddleware(hsts, "")(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// The seven headers that must always be present regardless of HSTS.
		want := map[string]string{
			"X-Content-Type-Options":       "nosniff",
			"X-Frame-Options":              "DENY",
			"Referrer-Policy":              "strict-origin-when-cross-origin",
			"Cross-Origin-Opener-Policy":   "same-origin",
			"Cross-Origin-Resource-Policy": "same-origin",
		}
		for h, v := range want {
			if got := rec.Header().Get(h); got != v {
				t.Errorf("hsts=%v: header %q = %q, want %q", hsts, h, got, v)
			}
		}
		// CSP and Permissions-Policy are present and non-empty with their
		// distinguishing directives.
		if csp := rec.Header().Get("Content-Security-Policy"); csp == "" {
			t.Errorf("hsts=%v: Content-Security-Policy must be set", hsts)
		} else if !containsAll(csp, "default-src 'self'", "frame-ancestors 'none'", "object-src 'none'") {
			t.Errorf("hsts=%v: CSP missing expected directives: %q", hsts, csp)
		}
		if pp := rec.Header().Get("Permissions-Policy"); pp == "" {
			t.Errorf("hsts=%v: Permissions-Policy must be set", hsts)
		} else if !containsAll(pp, "geolocation=()", "camera=()", "microphone=()") {
			t.Errorf("hsts=%v: Permissions-Policy missing expected features: %q", hsts, pp)
		}
	}
}

// FE Sentry posts events cross-origin, so connect-src must allow the DSN's
// ingest origin when configured — and stay tight ('self' only) when it isn't.
func TestSecurityHeadersMiddleware_ConnectSrcAllowsSentryIngest(t *testing.T) {
	t.Parallel()

	cspFor := func(dsn string) string {
		handler := SecurityHeadersMiddleware(false, dsn)(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
		)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		return rec.Header().Get("Content-Security-Policy")
	}

	if csp := cspFor(""); !strings.Contains(csp, "connect-src 'self'") ||
		strings.Contains(csp, "sentry.io") {
		t.Fatalf("empty DSN must keep connect-src tight: %q", csp)
	}

	if csp := cspFor("https://abc123@o42.ingest.de.sentry.io/7"); !strings.Contains(
		csp, "connect-src 'self' https://o42.ingest.de.sentry.io",
	) {
		t.Fatalf("DSN ingest origin must be in connect-src: %q", csp)
	}
}

func TestSecurityHeadersMiddleware_HSTSGatedOnFlag(t *testing.T) {
	t.Parallel()

	serve := func(hstsEnabled bool) *httptest.ResponseRecorder {
		handler := SecurityHeadersMiddleware(hstsEnabled, "")(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
		)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	on := serve(true)
	if got := on.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatal("HSTS header must be present when hstsEnabled=true")
	} else if !containsAll(got, "max-age=31536000", "includeSubDomains", "preload") {
		t.Fatalf("HSTS header missing expected attributes: %q", got)
	}

	off := serve(false)
	if got := off.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS header must be ABSENT when hstsEnabled=false, got %q", got)
	}
}

// --- presentation-18 + roadmap-82: CORSMiddleware allows cross-origin requests,
// short-circuits preflight OPTIONS with 204, and always adds Vary: Origin. ---

func TestCORSMiddleware_SetsAllowHeadersAndPassesThroughGET(t *testing.T) {
	t.Parallel()

	const origin = "http://localhost:5173"
	called := false
	handler := CORSMiddleware(
		origin,
	)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("GET request must reach the wrapped handler")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status: got %d want 200", rec.Code)
	}

	want := map[string]string{
		"Access-Control-Allow-Origin":      origin,
		"Access-Control-Allow-Methods":     "GET, POST, PUT, DELETE, OPTIONS",
		"Access-Control-Allow-Headers":     "Content-Type, Authorization",
		"Access-Control-Allow-Credentials": "true",
	}
	for h, v := range want {
		if got := rec.Header().Get(h); got != v {
			t.Errorf("header %q = %q, want %q", h, got, v)
		}
	}
}

func TestCORSMiddleware_PreflightOptionsShortCircuitsWith204(t *testing.T) {
	t.Parallel()

	called := false
	handler := CORSMiddleware("http://localhost:5173")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
		}),
	)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight OPTIONS: got %d want 204", rec.Code)
	}
	if called {
		t.Fatal("preflight OPTIONS must NOT reach the wrapped handler")
	}
	// Allow-Origin must still be set on the preflight response.
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("preflight Allow-Origin = %q, want origin", got)
	}
}

// roadmap-82: CORS responses include Vary: Origin so shared caches do not
// serve one origin's response to another.
func TestCORSMiddleware_AddsVaryOrigin(t *testing.T) {
	t.Parallel()

	// Both a normal request and a preflight must carry Vary: Origin.
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		handler := CORSMiddleware("http://localhost:5173")(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
		)
		req := httptest.NewRequest(method, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !slices.Contains(rec.Header().Values("Vary"), "Origin") {
			t.Fatalf(
				"%s: Vary header must contain Origin, got %v",
				method,
				rec.Header().Values("Vary"),
			)
		}
	}
}

// --- roadmap-77: IPMiddleware stashes the extracted client IP into ctx
// (consumed downstream by the audit middleware). ---

func TestIPMiddleware_StashesClientIPIntoContext(t *testing.T) {
	t.Parallel()

	var seen string
	// Default extractor: RemoteAddr host, ignores X-Real-IP.
	handler := IPMiddleware(NewIPExtractor(false))(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = shared.ActorIPFromContext(r.Context())
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	req.Header.Set("X-Real-IP", "1.2.3.4") // must be ignored without trust-proxy
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seen != "203.0.113.7" {
		t.Fatalf("ActorIPFromContext = %q, want %q (RemoteAddr host)", seen, "203.0.113.7")
	}
}

func TestIPMiddleware_TrustProxyStashesXRealIP(t *testing.T) {
	t.Parallel()

	var seen string
	handler := IPMiddleware(NewIPExtractor(true))(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = shared.ActorIPFromContext(r.Context())
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Real-IP", "198.51.100.9")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seen != "198.51.100.9" {
		t.Fatalf("trust-proxy ActorIPFromContext = %q, want X-Real-IP %q", seen, "198.51.100.9")
	}
}

// --- local helpers (unique names to avoid clashing with sibling test files) ---

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
