package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type recordingReporter struct{ count int }

func (r *recordingReporter) Capture(context.Context, error, ...slog.Attr) { r.count++ }
func (*recordingReporter) Flush(time.Duration) bool                       { return true }

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
