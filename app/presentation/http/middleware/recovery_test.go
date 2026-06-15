package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gokick/app/domain/shared"
)

type recordingReporter struct {
	count int
	attrs []slog.Attr
}

func (r *recordingReporter) Capture(_ context.Context, _ error, attrs ...slog.Attr) {
	r.count++
	r.attrs = attrs
}
func (*recordingReporter) Flush(time.Duration) bool { return true }

func (*recordingReporter) WithRequestScope(ctx context.Context) context.Context { return ctx }

func (*recordingReporter) ContinueTrace(ctx context.Context, _, _ string) context.Context {
	return ctx
}

// A panic escaping the handler becomes a 500, is logged, and is reported once —
// never leaking a stack to the client or silently dropping the connection.
func TestRecoveryMiddleware_PanicYields500AndReports(t *testing.T) {
	t.Parallel()
	rep := &recordingReporter{}
	h := RecoveryMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), rep)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic must yield 500, got %d", rec.Code)
	}
	if rep.count != 1 {
		t.Fatalf("reporter must capture the panic once, got %d", rep.count)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("500 body should be JSON, got Content-Type %q", ct)
	}
}

// The reporter receives a fixed whitelist — method, request PATH (no query),
// User-Agent — and nothing else from the request. The query string is dropped
// outright (a ?token=… param would otherwise leak), and Authorization and Cookie
// must never reach the error tracker; the Sentry adapter turns exactly these
// attrs into event.Request.
func TestRecoveryMiddleware_ReportsWhitelistedRequestAttrs(t *testing.T) {
	t.Parallel()
	rep := &recordingReporter{}
	h := RecoveryMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), rep)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}))

	// A credential smuggled into the query string must not survive into the report.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x?token=super-secret-query", nil)
	req.Header.Set("User-Agent", "test-agent/1.0")
	req.Header.Set("Authorization", "Bearer super-secret-token")
	req.Header.Set("Cookie", "session=super-secret-cookie")
	h.ServeHTTP(httptest.NewRecorder(), req)

	got := map[string]string{}
	for _, a := range rep.attrs {
		got[a.Key] = a.Value.String()
	}
	if got[shared.LogKeyMethod] != http.MethodPost {
		t.Fatalf("method attr: got %q want %q", got[shared.LogKeyMethod], http.MethodPost)
	}
	// Path only — the query (and any credential in it) is dropped.
	if got[shared.LogKeyURL] != "/api/v1/x" {
		t.Fatalf("url attr: got %q want %q", got[shared.LogKeyURL], "/api/v1/x")
	}
	if got[shared.LogKeyUserAgent] != "test-agent/1.0" {
		t.Fatalf("user_agent attr: got %q want %q", got[shared.LogKeyUserAgent], "test-agent/1.0")
	}
	// Defensive: no attr value may carry a secret header or query param, under any key.
	for k, v := range got {
		if strings.Contains(v, "super-secret") {
			t.Fatalf("secret leaked into the error tracker via attr %q = %q", k, v)
		}
	}
}

// A clean handler passes through untouched: no 500, no report.
func TestRecoveryMiddleware_CleanPassthrough(t *testing.T) {
	t.Parallel()
	rep := &recordingReporter{}
	h := RecoveryMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), rep)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("clean handler status passthrough: got %d", rec.Code)
	}
	if rep.count != 0 {
		t.Fatalf("clean handler must not report, got %d", rep.count)
	}
}

// http.ErrAbortHandler is the sanctioned way to abort a response — it must be
// re-panicked (so net/http aborts) and NOT reported as error-tracker noise.
func TestRecoveryMiddleware_RepanicsErrAbortHandlerWithoutReporting(t *testing.T) {
	t.Parallel()
	rep := &recordingReporter{}
	h := RecoveryMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), rep)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}))

	var repanicked any
	func() {
		defer func() { repanicked = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	}()

	if repanicked != http.ErrAbortHandler {
		t.Fatalf("ErrAbortHandler must be re-panicked, got %v", repanicked)
	}
	if rep.count != 0 {
		t.Fatalf("a sanctioned abort must not be reported, got %d", rep.count)
	}
}
