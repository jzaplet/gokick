package handler

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// discardLogger is a throwaway logger for SPA handler tests that don't assert on
// log output (the warn-and-degrade path has its own buffer-backed logger).
func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// injectRuntimeConfig must tolerate the realistic ways a template's <head> can
// be written — attributes, casing, whitespace — so a routine index.html edit in
// a fork can't silently drop the runtime config (incl. the FE Sentry DSN) the
// way an exact "<head>" match would. A document with no <head> (only <header>,
// or none) reports ok=false and is returned unchanged.
func TestInjectRuntimeConfig(t *testing.T) {
	t.Parallel()
	cfg := SPAConfig{SentryDSN: "https://k@example.com/1", SentryEnvironment: "production"}

	withHead := []struct{ name, html string }{
		{"bare head", `<!doctype html><html><head><title>x</title></head></html>`},
		{"head with attributes", `<html><head lang="cs"><title>x</title></head></html>`},
		{"uppercase head", `<HTML><HEAD></HEAD></HTML>`},
		{"head with whitespace before close", "<html><head\n class=\"a\"><title>x</title></head>"},
	}
	for _, c := range withHead {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, ok := injectRuntimeConfig([]byte(c.html), cfg)
			if !ok {
				t.Fatalf("expected injection into %q", c.html)
			}
			if !bytes.Contains(out, []byte(`name="gokick:sentry-dsn"`)) {
				t.Fatalf("meta not injected: %s", out)
			}
			lower := strings.ToLower(string(out))
			if strings.Index(lower, "<meta") < strings.Index(lower, "<head") {
				t.Fatalf("meta must be injected after the head open tag: %s", out)
			}
		})
	}

	noHead := []struct{ name, html string }{
		{"only header element", `<html><body><header>nav</header></body></html>`},
		{"no head at all", `<html><body>hi</body></html>`},
	}
	for _, c := range noHead {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, ok := injectRuntimeConfig([]byte(c.html), cfg)
			if ok {
				t.Fatalf("must not inject when there is no <head>: %q", c.html)
			}
			if !bytes.Equal(out, []byte(c.html)) {
				t.Fatal("a no-head document must be returned unchanged")
			}
		})
	}
}

// A real index.html with no <head> anchor must WARN (so the missing telemetry is
// visible) yet still SERVE the page — a template edit in a fork degrades the FE
// runtime config to its build-time fallback, it does not crash the app.
func TestNewSPAHandler_WarnsButServesWhenNoHead(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html><body>no head here</body></html>")},
	}

	h := NewSPAHandler(logger, fsys, SPAConfig{SentryDSN: "https://k@example.com/1"})

	rec := httptest.NewRecorder()
	h.Serve(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buf.String(), "no <head>") {
		t.Fatalf("must warn about the missing <head>, got log: %q", buf.String())
	}
	if !strings.Contains(rec.Body.String(), "no head here") {
		t.Fatalf("must still serve the page (degraded), got: %q", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "gokick:sentry-dsn") {
		t.Fatal("a no-head document must NOT have meta injected")
	}
}
