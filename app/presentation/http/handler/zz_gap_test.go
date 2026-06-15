package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// presentation-30: GET /health is the health check. The handler must answer
// 200 OK with a JSON body of exactly {"status":"ok"}. The server-package
// routing test already pins the 200 status reached through the mux; what is
// pinned nowhere else is the body contract, so this test asserts the decoded
// payload (mutating the literal "ok" or the status code in health.go falsifies
// it).
func TestHealthHandler_Check_Returns200WithOkBody(t *testing.T) {
	h := NewHealthHandler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Check(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health body %q: %v", rec.Body.String(), err)
	}
	if body["status"] != "ok" {
		t.Fatalf(`health body: got %v want {"status":"ok"}`, body)
	}
}

// presentation-45: the SPA catch-all serves files from the embedded FS, and
// nonexistent files return 404. The two branches in SPAHandler.Serve are keyed
// on whether the request path contains a "." — a dotted path is delegated to
// the FileServer (404 when the file is absent), a dotless path falls back to
// index.html with 200. The server-package routing tests never exercise this
// handler, so all three branches below are otherwise unpinned.
func TestSPAHandler_Serve(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>spa-root</title>")},
		"app.js":     {Data: []byte("console.log('hello')")},
	}
	h := NewSPAHandler(discardLogger(), fsys, SPAConfig{})

	t.Run("existing dotted asset is served from the FS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		rec := httptest.NewRecorder()
		h.Serve(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d want 200", rec.Code)
		}
		if got := rec.Body.String(); got != "console.log('hello')" {
			t.Fatalf("asset body: got %q want the app.js contents", got)
		}
	})

	t.Run("missing dotted path returns 404", func(t *testing.T) {
		// A "." in the path routes to the FileServer branch; the file is
		// absent from the MapFS, so it must 404 (not fall back to index).
		req := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
		rec := httptest.NewRecorder()
		h.Serve(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("missing asset: got %d want 404; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("dotless route falls back to index.html", func(t *testing.T) {
		// No "." → SPA fallback branch: index.html at 200 regardless of
		// whether a matching file exists.
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		rec := httptest.NewRecorder()
		h.Serve(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("spa fallback: got %d want 200", rec.Code)
		}
		if got := rec.Body.String(); got != "<!doctype html><title>spa-root</title>" {
			t.Fatalf("spa fallback body: got %q want index.html contents", got)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("spa fallback Content-Type: got %q want text/html; charset=utf-8", ct)
		}
	})
}

// The runtime frontend config (Sentry DSN, environment, debug flag) is injected
// into the served index.html as <meta> tags so one built image serves every
// environment. The debug tag is emitted only when enabled.
func TestSPAHandler_InjectsRuntimeConfig(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte(
			"<!doctype html><html><head><title>x</title></head><body></body></html>",
		)},
	}
	h := NewSPAHandler(discardLogger(), fsys, SPAConfig{
		SentryDSN:         "https://k@o1.ingest.sentry.io/2",
		SentryEnvironment: "production",
		SentryDebug:       true,
	})

	rec := httptest.NewRecorder()
	h.Serve(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`<meta name="gokick:sentry-dsn" content="https://k@o1.ingest.sentry.io/2">`,
		`<meta name="gokick:sentry-environment" content="production">`,
		`<meta name="gokick:sentry-debug" content="true">`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("served index missing %q in:\n%s", want, body)
		}
	}
}

func TestSPAHandler_OmitsDebugMetaWhenDisabled(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<head></head>")},
	}
	h := NewSPAHandler(discardLogger(), fsys, SPAConfig{SentryDebug: false})

	rec := httptest.NewRecorder()
	h.Serve(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Contains(rec.Body.String(), "sentry-debug") {
		t.Fatalf("debug meta must be omitted when disabled: %s", rec.Body.String())
	}
}
