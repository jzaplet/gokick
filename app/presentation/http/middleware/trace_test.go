package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gokick/app/domain/shared"
)

func captureTraceID(t *testing.T, header string) string {
	t.Helper()
	var seen string
	mw := TraceMiddleware()
	handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = shared.TraceIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if header != "" {
		req.Header.Set("X-Trace-Id", header)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Trace-Id"); got != seen {
		t.Fatalf("response trace_id %q != context trace_id %q", got, seen)
	}
	return seen
}

func TestTraceMiddleware_AcceptsValidInboundID(t *testing.T) {
	id := "abcd-1234-EFGH_5678"
	if got := captureTraceID(t, id); got != id {
		t.Fatalf("trace_id should pass through valid value: got %q want %q", got, id)
	}
}

func TestTraceMiddleware_RejectsLogInjectionAttempt(t *testing.T) {
	// Newlines + control chars would let a caller inject fake log lines.
	got := captureTraceID(t, "valid-looking\nINJECT log_level=ERROR msg=fake")
	if got == "" {
		t.Fatal("middleware must always populate a trace_id")
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("trace_id leaked control chars: %q", got)
	}
}

func TestTraceMiddleware_RejectsTooShortID(t *testing.T) {
	got := captureTraceID(t, "abc")
	if got == "abc" {
		t.Fatal("trace_id below the min length must be replaced, not echoed")
	}
}

func TestTraceMiddleware_GeneratesWhenMissing(t *testing.T) {
	got := captureTraceID(t, "")
	if got == "" {
		t.Fatal("trace_id must be generated when header is absent")
	}
	if len(got) < 8 {
		t.Fatalf("generated trace_id looks too short: %q", got)
	}
}
