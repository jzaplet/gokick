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

	"gokick/app/presentation/http/response"
)

// discardLogger is a throwaway logger for SPA handler tests that don't assert on
// log output (the warn-and-degrade path has its own buffer-backed logger).
func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// testResponder is a Responder over a discard logger for handler tests that
// don't assert on the (rare) encode-failure log line.
func testResponder() *response.Responder { return response.NewResponder(discardLogger()) }

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

// Beyond casing/attributes, the head matcher must find the real tag end past a
// '>' inside a quoted attribute value rather than splicing the config mid-tag.
// (The matcher does not parse HTML comments — a "<head" inside a leading comment
// would be matched; see headInsertPos. That is an accepted limitation for a
// Vite-built index.html, so it is deliberately not asserted here.)
func TestInjectRuntimeConfig_RobustEdges(t *testing.T) {
	t.Parallel()
	cfg := SPAConfig{SentryDSN: "https://k@example.com/1"}

	t.Run("'>' inside a quoted attribute is not the tag end", func(t *testing.T) {
		t.Parallel()
		out, ok := injectRuntimeConfig(
			[]byte(`<html><head data-x="a>b"><title>x</title></head></html>`), cfg)
		s := string(out)
		if !ok {
			t.Fatalf("must inject: %s", s)
		}
		if !strings.Contains(s, `data-x="a>b"><meta`) {
			t.Fatalf("meta must go after the real tag end, past the quoted '>': %s", s)
		}
	})
}

// An unknown /api path must NOT fall through to the SPA index — it returns a
// JSON 404 (a 200 text/html page would surface client-side as a confusing parse
// error). A dotless non-/api path still serves the SPA; a dotted path still hits
// the file server.
func TestSPAHandler_Serve_Routing(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html><head></head><body>app</body></html>")},
		"app.js":     &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	h := NewSPAHandler(testResponder(), discardLogger(), fsys, SPAConfig{})

	t.Run("unknown /api path is a JSON 404", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		h.Serve(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("want JSON content-type, got %q", ct)
		}
		if strings.Contains(rec.Body.String(), "<html") {
			t.Fatalf("must not serve the SPA index for /api, got: %s", rec.Body.String())
		}
	})

	t.Run("dotless SPA route serves index 200", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		h.Serve(rec, httptest.NewRequest(http.MethodGet, "/admin/users", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app") {
			t.Fatalf("SPA route must serve index 200, got %d body=%q", rec.Code, rec.Body.String())
		}
	})

	t.Run("dotted asset hits the file server", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		h.Serve(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log") {
			t.Fatalf(
				"asset must be served by the file server, got %d body=%q",
				rec.Code,
				rec.Body.String(),
			)
		}
	})
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

	h := NewSPAHandler(
		testResponder(),
		logger,
		fsys,
		SPAConfig{SentryDSN: "https://k@example.com/1"},
	)

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
