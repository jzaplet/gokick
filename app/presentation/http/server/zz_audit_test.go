package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/internal/testfx"
	"gokick/app/presentation/http/handler"
	"gokick/app/presentation/http/middleware"
)

// silentLogger returns a slog.Logger that discards everything — keeps test
// output clean while the real middleware logging code still executes.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// chainOnlyServer builds a Server populated with just the fields
// buildMiddlewareChain touches (config, logger, ipExtract). The route/handler
// fields stay nil — buildMiddlewareChain never dereferences them.
func chainOnlyServer(cookieSecure bool) *Server {
	return &Server{
		config: &config.Config{
			CookieSecure: cookieSecure,
			CORSOrigin:   "https://app.example.com",
		},
		logger:    silentLogger(),
		reporter:  shared.NopReporter{},
		ipExtract: middleware.NewIPExtractor(false),
	}
}

// TestBuildMiddlewareChain_TraceRunsBeforeHandlerAndHeadersApplied pins the
// observable effects of the documented chain (Trace -> IP -> Security -> CORS
// -> CSRF -> Logging -> handler): the handler sees a trace ID already in
// context (Trace ran before it), and the response carries the security +
// CORS header effects. Closes the chain-wiring half of overview-13 /
// presentation-21 / presentation-22.
func TestBuildMiddlewareChain_TraceRunsBeforeHandlerAndHeadersApplied(t *testing.T) {
	t.Parallel()

	var seenTraceID string
	var handlerRan bool
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerRan = true
		// Trace middleware must run BEFORE the terminal handler, so the
		// resolved trace ID is already in context here.
		seenTraceID = shared.TraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	s := chainOnlyServer(false)
	chain := s.buildMiddlewareChain(sentinel)

	// GET (not OPTIONS) so CORS passes through to the handler instead of
	// short-circuiting with 204.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	if !handlerRan {
		t.Fatal("terminal handler never ran through the chain")
	}
	if seenTraceID == "" {
		t.Fatal("handler saw empty trace ID — Trace middleware did not run before the handler")
	}

	// Trace middleware echoes the resolved ID on the response.
	if got := rec.Header().Get("X-Trace-Id"); got == "" {
		t.Fatal("response missing X-Trace-Id — Trace middleware not in chain")
	} else if got != seenTraceID {
		t.Fatalf("trace ID drift: response header %q vs context %q", got, seenTraceID)
	}

	// SecurityHeadersMiddleware effects.
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options: got %q want nosniff — security headers not in chain", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options: got %q want DENY — security headers not in chain", got)
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing Content-Security-Policy — security headers not in chain")
	}

	// CORSMiddleware effect — origin reflected from config.
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf(
			"Access-Control-Allow-Origin: got %q want configured origin — CORS not in chain",
			got,
		)
	}
}

// TestBuildMiddlewareChain_HSTSGatedByCookieSecure proves the server wires the
// CookieSecure flag into SecurityHeadersMiddleware's HSTS gate: present in
// production (CookieSecure=true), absent otherwise. Closes infra-config-wire-12.
func TestBuildMiddlewareChain_HSTSGatedByCookieSecure(t *testing.T) {
	t.Parallel()

	fire := func(cookieSecure bool) string {
		s := chainOnlyServer(cookieSecure)
		chain := s.buildMiddlewareChain(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		)
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "203.0.113.7:5555"
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		return rec.Header().Get("Strict-Transport-Security")
	}

	if hsts := fire(true); hsts == "" {
		t.Fatal("CookieSecure=true must emit Strict-Transport-Security, got none")
	}
	if hsts := fire(false); hsts != "" {
		t.Fatalf("CookieSecure=false must NOT emit Strict-Transport-Security, got %q", hsts)
	}
}

// routingServer builds a Server with the minimal real dependencies that
// registerRoutes dereferences: jwt (for the AuthMiddleware wrapper), the two
// rate limiters, and real health + SPA handlers. The bus-backed handlers stay
// nil — registerRoutes only forms method values from them (safe on a nil
// pointer); they are never *called* in these tests because the invalid-Bearer
// guard short-circuits protected routes with 401 before the handler, and the
// probed public route is /health.
func routingServer(t *testing.T) *Server {
	t.Helper()

	jwt := testfx.NewJwt(t, 15*time.Minute)
	logger := silentLogger()
	extract := middleware.NewIPExtractor(false)

	// Generous, never-exhausted bucket so the limiter middleware is a true
	// pass-through for our single-shot probes (and Per != 0 avoids the
	// div-by-zero in NewRateLimiter's refill calc).
	rule := middleware.RateRule{Tokens: 1000, Per: time.Minute}

	return &Server{
		config:    &config.Config{CookieSecure: false, CORSOrigin: "*"},
		logger:    logger,
		reporter:  shared.NopReporter{},
		jwt:       jwt,
		ipExtract: extract,
		limiters: &RateLimiters{
			Login:   middleware.NewRateLimiter(rule, extract, logger),
			Refresh: middleware.NewRateLimiter(rule, extract, logger),
		},
		health: handler.NewHealthHandler(),
		// fstest.MapFS has no index.html; NewSPAHandler falls back to a
		// built-in default index, so the catch-all never panics.
		spa: handler.NewSPAHandler(fstest.MapFS{}, handler.SPAConfig{}),
	}
}

// TestRegisterRoutes_ProtectedRoutesRejectInvalidBearer proves every protected
// route is registered at the documented method+path AND wrapped in
// AuthMiddleware: an invalid Bearer token is rejected with 401 by the route
// guard before the (nil) handler runs. Covers the route-presence + auth-guard
// claims: guide-auth-perm-17 (logout), overview-12 / overview-21 / presentation-39
// / presentation-40 / presentation-41 (admin users routes), presentation-37 /
// presentation-38 (dashboard routes), and the protected half of overview-13 /
// presentation-22. NOTE: this asserts registration + protection only, not which
// command/query each route dispatches nor its permission string (those are
// covered by the *_RequiredPermission tests in the command/query packages).
func TestRegisterRoutes_ProtectedRoutesRejectInvalidBearer(t *testing.T) {
	t.Parallel()

	mux := routingServer(t).registerRoutes()

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/profile"},
		{http.MethodPut, "/api/v1/profile/password"},
		{http.MethodGet, "/api/v1/dashboard/user"},
		{http.MethodGet, "/api/v1/dashboard/admin"},
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodPost, "/api/v1/admin/users"},
		{http.MethodPut, "/api/v1/admin/users/abc123"},
		{http.MethodDelete, "/api/v1/admin/users/abc123"},
	}

	for _, tc := range protected {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			// A *present but invalid* Bearer is the discriminator: a MISSING
			// header passes AuthMiddleware through (would hit the nil handler
			// and panic), so we must send garbage to trigger the 401 path.
			req.Header.Set("Authorization", "Bearer not-a-real-token")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf(
					"%s %s with invalid Bearer: got %d want 401 (route missing or not auth-wrapped); body=%s",
					tc.method,
					tc.path,
					rec.Code,
					rec.Body.String(),
				)
			}
		})
	}
}

// TestRegisterRoutes_PublicRoutesSkipAuth proves the public/protected split:
// /health carries no AuthMiddleware, so even an invalid Bearer reaches the
// handler and returns 200 (an auth-wrapped route would 401 the same request).
// Closes the public-route half of presentation-21 / overview-13.
func TestRegisterRoutes_PublicRoutesSkipAuth(t *testing.T) {
	t.Parallel()

	mux := routingServer(t).registerRoutes()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"GET /health with invalid Bearer: got %d want 200 — /health must NOT be auth-wrapped; body=%s",
			rec.Code,
			rec.Body.String(),
		)
	}
}

// TestRegisterRoutes_MethodSpecificRouting gives negative coverage that the
// routes are registered per-method (Go 1.22 method-aware patterns): a method
// that no pattern covers for a known path yields 405, not a fall-through to
// the SPA catch-all. /health is registered for GET only.
func TestRegisterRoutes_MethodSpecificRouting(t *testing.T) {
	t.Parallel()

	mux := routingServer(t).registerRoutes()

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /health: got %d want 405 — routes must be method-specific; body=%s",
			rec.Code, rec.Body.String())
	}
}
