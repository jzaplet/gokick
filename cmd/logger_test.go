package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestNewLogHandler_JSONByDefault(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(newLogHandler(&buf, "", slog.LevelInfo))
	logger.Info("hello", "k", "v")

	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("empty format must default to valid JSON: %v (out=%q)", err, buf.String())
	}
	if m["msg"] != "hello" || m["k"] != "v" {
		t.Fatalf("unexpected JSON log: %v", m)
	}
}

func TestNewLogHandler_TextFormat(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(newLogHandler(&buf, "TEXT", slog.LevelInfo)) // case-insensitive
	logger.Info("hello", "k", "v")

	out := strings.TrimSpace(buf.String())
	if strings.HasPrefix(out, "{") {
		t.Fatalf("text format must not emit JSON: %q", out)
	}
	if !strings.Contains(out, "msg=hello") || !strings.Contains(out, "k=v") {
		t.Fatalf("unexpected text log: %q", out)
	}
}

func TestNewLogHandler_RespectsLevel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(newLogHandler(&buf, "json", slog.LevelWarn))
	logger.Info("suppressed")
	logger.Warn("kept")

	out := buf.String()
	if strings.Contains(out, "suppressed") {
		t.Fatalf("Info must be filtered out at Warn level: %q", out)
	}
	if !strings.Contains(out, "kept") {
		t.Fatalf("Warn must pass at Warn level: %q", out)
	}
}

// breadcrumbHandler must stay a transparent logging pass-through: it writes the
// record to the wrapped handler whether or not a Sentry hub is on the context
// (no hub → no breadcrumb, no panic). The breadcrumb side-effect itself is
// verified end-to-end in sentry_test.go.
func TestBreadcrumbHandler_PassThroughWithoutHub(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(breadcrumbHandler{Handler: newLogHandler(&buf, "json", slog.LevelInfo)})

	logger.InfoContext(context.Background(), "no-hub", "k", "v")

	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("wrapped handler must still emit valid JSON: %v (out=%q)", err, buf.String())
	}
	if m["msg"] != "no-hub" || m["k"] != "v" {
		t.Fatalf("wrapped handler dropped fields: %v", m)
	}
}

// logger.With must keep the breadcrumb wrapper (WithAttrs re-wraps), so bound
// attrs still log and the breadcrumb behaviour survives a derived logger.
func TestBreadcrumbHandler_SurvivesWith(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	base := slog.New(breadcrumbHandler{Handler: newLogHandler(&buf, "json", slog.LevelInfo)})
	logger := base.With("bound", "yes")

	if _, ok := logger.Handler().(breadcrumbHandler); !ok {
		t.Fatalf("With must preserve breadcrumbHandler, got %T", logger.Handler())
	}
	logger.Info("msg")
	if !strings.Contains(buf.String(), `"bound":"yes"`) {
		t.Fatalf("bound attr missing after With: %q", buf.String())
	}
}

// recordToBreadcrumb must flatten an inline slog.Group into dotted child keys
// (mirroring the text handler) rather than storing an opaque []slog.Attr that
// JSON-marshals to {} — so the breadcrumb trail keeps the nested fields.
func TestRecordToBreadcrumb_FlattensInlineGroup(t *testing.T) {
	t.Parallel()
	h := breadcrumbHandler{Handler: newLogHandler(io.Discard, "json", slog.LevelInfo)}
	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "msg", 0)
	r.Add("flat", "v", slog.Group("g", slog.String("k1", "v1"), slog.Int("k2", 2)))

	data := h.recordToBreadcrumb(r).Data

	if data["flat"] != "v" {
		t.Fatalf("flat attr: got %v", data["flat"])
	}
	if data["g.k1"] != "v1" {
		t.Fatalf("group child g.k1 must be flattened, got %v (data=%v)", data["g.k1"], data)
	}
	if data["g.k2"] != int64(2) {
		t.Fatalf("group child g.k2: got %v (%T)", data["g.k2"], data["g.k2"])
	}
}

func TestParseLogLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"":        slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"bogus":   slog.LevelInfo,
		"  warn ": slog.LevelWarn,
	}
	for in, want := range cases {
		if got := parseLogLevel(in); got != want {
			t.Errorf("parseLogLevel(%q): got %v want %v", in, got, want)
		}
	}
}
