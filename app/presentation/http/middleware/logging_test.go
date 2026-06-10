package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"gokick/app/domain/shared"
)

// The HTTP access log carries trace_id, method, path and a numeric duration_ms.
// This is a behavior change (was: trace_id + a stringly time.Duration), so the
// emitted shape is locked here.
func TestLoggingMiddleware_LogsRequestShape(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = req.WithContext(shared.ContextWithTraceID(req.Context(), "trace-77"))
	h.ServeHTTP(httptest.NewRecorder(), req)

	entry := decodeSingleLog(t, buf.Bytes())
	if entry["msg"] != "http: request" {
		t.Fatalf("msg: got %v", entry["msg"])
	}
	if entry[shared.LogKeyTraceID] != "trace-77" {
		t.Fatalf("trace_id: got %v", entry[shared.LogKeyTraceID])
	}
	if entry["method"] != "GET" || entry["path"] != "/dashboard" {
		t.Fatalf("method/path: got %v %v", entry["method"], entry["path"])
	}
	if _, ok := entry[shared.LogKeyDurationMs].(float64); !ok {
		t.Fatalf("duration_ms must be a JSON number, got %T (%v)",
			entry[shared.LogKeyDurationMs], entry[shared.LogKeyDurationMs])
	}
}

// Structural lock: the global access log runs OUTSIDE per-route AuthMiddleware,
// so claims injected downstream never reach it — the HTTP line carries trace_id
// but NOT user_id (identity lives on the bus log line for the same trace_id).
// A middleware reordering that broke this assumption would flip this assertion.
func TestLoggingMiddleware_OmitsUserIDEvenWhenClaimsInjectedDownstream(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	deepest := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// authish mimics AuthMiddleware: claims go into a child ctx that flows only
	// downward (to deepest), never back up to the outer LoggingMiddleware.
	authish := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claimsCtx := shared.ContextWithClaims(r.Context(), &shared.AuthClaims{UserID: "u-1"})
		deepest.ServeHTTP(w, r.WithContext(claimsCtx))
	})
	h := LoggingMiddleware(logger)(authish)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req = req.WithContext(shared.ContextWithTraceID(req.Context(), "trace-1"))
	h.ServeHTTP(httptest.NewRecorder(), req)

	entry := decodeSingleLog(t, buf.Bytes())
	if _, present := entry[shared.LogKeyUserID]; present {
		t.Fatalf(
			"user_id must be absent on the HTTP access log line, got %v",
			entry[shared.LogKeyUserID],
		)
	}
	if entry[shared.LogKeyTraceID] != "trace-1" {
		t.Fatalf("trace_id must be present: %v", entry)
	}
}

func decodeSingleLog(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if bytes.Contains(trimmed, []byte("\n")) {
		t.Fatalf("expected exactly one log line, got: %q", trimmed)
	}
	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err != nil {
		t.Fatalf("invalid JSON log line %q: %v", trimmed, err)
	}
	return m
}
