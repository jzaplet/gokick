package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/presentation/http/middleware"
)

// ctxRecordingReporter stashes the ctx of the most recent Capture so a test can
// assert what request-scoped values were in scope when a panic was reported.
type ctxRecordingReporter struct{ ctx context.Context }

func (r *ctxRecordingReporter) Capture(ctx context.Context, _ error, _ ...slog.Attr) {
	r.ctx = ctx
}
func (*ctxRecordingReporter) Flush(time.Duration) bool { return true }

// TestBuildMiddlewareChain_RecoveryCaptureSeesClientIP pins the ordering
// invariant that IPMiddleware runs BEFORE RecoveryMiddleware: when a handler
// panics, the ctx the error reporter receives must already carry the resolved
// client IP, so Sentry can attribute the panic to an origin (user.ip_address).
//
// This is asserted against buildMiddlewareChain — not a hand-wired stack —
// because the ordering invariant lives in that method; a hand-wired chain would
// not catch a future reorder of server.go. The test FAILS if Recovery is moved
// back outside IPMiddleware (the captured ctx would lack the IP, which is the
// exact gap a live v0.0.4 Sentry event surfaced).
func TestBuildMiddlewareChain_RecoveryCaptureSeesClientIP(t *testing.T) {
	t.Parallel()

	rep := &ctxRecordingReporter{}
	s := &Server{
		config:   &config.Config{CookieSecure: false, CORSOrigin: "*"},
		logger:   silentLogger(),
		reporter: rep,
		// trustProxy=true so the extractor honours CF-Connecting-IP — the real
		// path behind Cloudflare, and what the live deploy exercises.
		ipExtract: middleware.NewIPExtractor(true),
	}

	chain := s.buildMiddlewareChain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/debug", nil)
	req.Header.Set("CF-Connecting-IP", "203.0.113.42")
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic must yield 500, got %d", rec.Code)
	}
	if rep.ctx == nil {
		t.Fatal("reporter never received a capture — RecoveryMiddleware not in chain")
	}
	if got := shared.ActorIPFromContext(rep.ctx); got != "203.0.113.42" {
		t.Fatalf(
			"captured ctx client IP: got %q want %q — IPMiddleware must run before RecoveryMiddleware",
			got,
			"203.0.113.42",
		)
	}
}
