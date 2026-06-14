package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
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
