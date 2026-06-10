package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"gokick/app/domain/shared"
)

// An authenticated command logs the full correlation set (trace_id, user_id,
// command) plus a numeric duration_ms — the fields a log aggregator queries on.
func TestLoggingMiddleware_EmitsCorrelationAndDurationMs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mw := LoggingMiddleware(slog.New(slog.NewJSONHandler(&buf, nil)))

	ctx := shared.ContextWithTraceID(context.Background(), "trace-123")
	ctx = shared.ContextWithClaims(ctx, &shared.AuthClaims{UserID: "user-7"})

	_, err := mw(ctx, "CreateUser", normalCmd{}, func(context.Context) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	completed := findLogLine(t, decodeLogLines(t, buf.Bytes()), "bus: completed")
	if completed[shared.LogKeyTraceID] != "trace-123" {
		t.Fatalf("trace_id: got %v", completed[shared.LogKeyTraceID])
	}
	if completed[shared.LogKeyUserID] != "user-7" {
		t.Fatalf("user_id: got %v", completed[shared.LogKeyUserID])
	}
	if completed[shared.LogKeyCommand] != "CreateUser" {
		t.Fatalf("command: got %v", completed[shared.LogKeyCommand])
	}
	durMs, ok := completed[shared.LogKeyDurationMs].(float64)
	if !ok {
		t.Fatalf("duration_ms must be a JSON number, got %T (%v)",
			completed[shared.LogKeyDurationMs], completed[shared.LogKeyDurationMs])
	}
	if durMs < 0 {
		t.Fatalf("duration_ms must be non-negative, got %v", durMs)
	}
}

// login/refresh run unauthenticated (SkipPermission) — there are no claims in
// ctx, so user_id must be omitted rather than logged empty.
func TestLoggingMiddleware_OmitsUserIDWhenUnauthenticated(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mw := LoggingMiddleware(slog.New(slog.NewJSONHandler(&buf, nil)))

	ctx := shared.ContextWithTraceID(context.Background(), "trace-xyz")
	_, _ = mw(ctx, "Login", normalCmd{}, func(context.Context) (any, error) {
		return nil, nil
	})

	lines := decodeLogLines(t, buf.Bytes())
	if len(lines) == 0 {
		t.Fatal("expected at least one log line")
	}
	for _, entry := range lines {
		if _, present := entry[shared.LogKeyUserID]; present {
			t.Fatalf("user_id must be absent for unauthenticated commands, line: %v", entry)
		}
		if entry[shared.LogKeyTraceID] != "trace-xyz" {
			t.Fatalf("trace_id must still be present, line: %v", entry)
		}
	}
}

// A failing handler's error is logged on the "bus: failed" line (with key
// "error") and propagated unchanged to the caller; duration_ms is still emitted.
func TestLoggingMiddleware_FailureLogsErrorAndDuration(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mw := LoggingMiddleware(slog.New(slog.NewJSONHandler(&buf, nil)))

	wantErr := errors.New("boom")
	ctx := shared.ContextWithTraceID(context.Background(), "trace-9")
	_, err := mw(ctx, "DoThing", normalCmd{}, func(context.Context) (any, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("middleware must pass the handler error through, got %v", err)
	}

	failed := findLogLine(t, decodeLogLines(t, buf.Bytes()), "bus: failed")
	if failed[shared.LogKeyError] != "boom" {
		t.Fatalf("error attr: got %v", failed[shared.LogKeyError])
	}
	if _, ok := failed[shared.LogKeyDurationMs].(float64); !ok {
		t.Fatalf("duration_ms missing on failure line: %v", failed)
	}
}

func decodeLogLines(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func findLogLine(t *testing.T, entries []map[string]any, msg string) map[string]any {
	t.Helper()
	for _, e := range entries {
		if e["msg"] == msg {
			return e
		}
	}
	t.Fatalf("no log line with msg %q in %v", msg, entries)
	return nil
}
