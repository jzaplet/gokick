package main

import (
	"bytes"
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
