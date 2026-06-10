package shared

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestLogAttrs_EmptyContext(t *testing.T) {
	t.Parallel()
	if attrs := LogAttrs(context.Background()); len(attrs) != 0 {
		t.Fatalf("background ctx must yield no attrs, got %d: %v", len(attrs), attrs)
	}
}

func TestLogAttrs_TraceOnly(t *testing.T) {
	t.Parallel()
	ctx := ContextWithTraceID(context.Background(), "trace-abc")
	attrs := LogAttrs(ctx)
	if len(attrs) != 1 {
		t.Fatalf("want 1 attr, got %d: %v", len(attrs), attrs)
	}
	assertLogAttrString(t, attrs[0], LogKeyTraceID, "trace-abc")
}

func TestLogAttrs_TraceAndUser(t *testing.T) {
	t.Parallel()
	ctx := ContextWithTraceID(context.Background(), "trace-abc")
	ctx = ContextWithClaims(ctx, &AuthClaims{UserID: "user-1", Role: "admin"})
	attrs := LogAttrs(ctx)
	if len(attrs) != 2 {
		t.Fatalf("want 2 attrs, got %d: %v", len(attrs), attrs)
	}
	assertLogAttrString(t, attrs[0], LogKeyTraceID, "trace-abc")
	assertLogAttrString(t, attrs[1], LogKeyUserID, "user-1")
}

func TestLogAttrs_UserWithoutTrace(t *testing.T) {
	t.Parallel()
	ctx := ContextWithClaims(context.Background(), &AuthClaims{UserID: "user-9"})
	attrs := LogAttrs(ctx)
	if len(attrs) != 1 {
		t.Fatalf("want 1 attr, got %d: %v", len(attrs), attrs)
	}
	assertLogAttrString(t, attrs[0], LogKeyUserID, "user-9")
}

func TestLogAttrs_EmptyUserIDOmitted(t *testing.T) {
	t.Parallel()
	// Claims present but UserID empty (shouldn't happen, but be defensive) —
	// an empty user_id is noise, so it must be omitted.
	ctx := ContextWithClaims(context.Background(), &AuthClaims{Role: "user"})
	if attrs := LogAttrs(ctx); len(attrs) != 0 {
		t.Fatalf("empty UserID must be omitted, got %v", attrs)
	}
}

func TestLogAttrs_FreshSliceNoAliasing(t *testing.T) {
	t.Parallel()
	ctx := ContextWithTraceID(context.Background(), "t1")
	// Each call must return an independent slice so a caller appending its own
	// per-line attrs never corrupts another line's.
	a := append(LogAttrs(ctx), slog.String("x", "1"))
	b := append(LogAttrs(ctx), slog.String("y", "2"))
	assertLogAttrString(t, a[len(a)-1], "x", "1")
	assertLogAttrString(t, b[len(b)-1], "y", "2")
}

func TestDurationMsAttr_Key(t *testing.T) {
	t.Parallel()
	attr := DurationMsAttr(1500 * time.Microsecond) // 1.5 ms
	if attr.Key != LogKeyDurationMs {
		t.Fatalf("key: got %q want %q", attr.Key, LogKeyDurationMs)
	}
	if got := attr.Value.Float64(); got != 1.5 {
		t.Fatalf("value: got %v want 1.5", got)
	}
}

func TestDurationMsAttr_SubMillisecondPrecision(t *testing.T) {
	t.Parallel()
	// 333µs must not round to 0 — fractional ms is the whole point.
	if got := DurationMsAttr(333 * time.Microsecond).Value.Float64(); got != 0.333 {
		t.Fatalf("333µs must log as 0.333 ms, got %v", got)
	}
}

func TestMillisAttr_CustomKey(t *testing.T) {
	t.Parallel()
	attr := MillisAttr(LogKeyRetryInMs, 5*time.Second)
	if attr.Key != LogKeyRetryInMs {
		t.Fatalf("key: got %q want %q", attr.Key, LogKeyRetryInMs)
	}
	if got := attr.Value.Float64(); got != 5000 {
		t.Fatalf("value: got %v want 5000", got)
	}
}

// The standardized key names are an external contract — log aggregator
// (Loki/Grafana) queries and dashboards hard-code them. Pin each constant to
// its exact wire string so a rename is a conscious, test-breaking act rather
// than a silent change that keeps every symbolic assertion green.
func TestLogKeyConstants_WireValues(t *testing.T) {
	t.Parallel()
	cases := []struct{ got, want string }{
		{LogKeyTraceID, "trace_id"},
		{LogKeyUserID, "user_id"},
		{LogKeyCommand, "command"},
		{LogKeyDurationMs, "duration_ms"},
		{LogKeyRetryInMs, "retry_in_ms"},
		{LogKeyError, "error"},
		{LogKeyEvent, "event"},
		{LogKeyJobKind, "job_kind"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("log key constant = %q, want %q", c.got, c.want)
		}
	}
}

func assertLogAttrString(t *testing.T, attr slog.Attr, key, val string) {
	t.Helper()
	if attr.Key != key {
		t.Fatalf("attr key: got %q want %q", attr.Key, key)
	}
	if got := attr.Value.String(); got != val {
		t.Fatalf("attr %q value: got %q want %q", key, got, val)
	}
}
