package handler

import (
	"bytes"
	"html"
	"io/fs"
	"net/http"
	"strings"
)

// Meta-tag names carrying the runtime frontend config. The Go server injects
// these into index.html at serve time; the SPA reads them at runtime. Meta tags
// (not an inline <script>) are used because the CSP is script-src 'self' — an
// inline script would be blocked, whereas a meta tag carries no executable code
// and is read via the DOM.
const (
	metaSentryDSN         = "gokick:sentry-dsn"
	metaSentryEnvironment = "gokick:sentry-environment"
	metaSentryDebug       = "gokick:sentry-debug"
)

// SPAConfig is the deployment-specific frontend config injected into the served
// HTML. It is a focused value (not the whole *config.Config) so the handler
// layer stays free of an infrastructure/config import; the DI layer builds it.
type SPAConfig struct {
	SentryDSN         string
	SentryEnvironment string
	SentryDebug       bool
}

type SPAHandler struct {
	fs    http.Handler
	index []byte
}

func NewSPAHandler(publicFS fs.FS, cfg SPAConfig) *SPAHandler {
	index, err := fs.ReadFile(publicFS, "index.html")
	if err != nil {
		index = []byte(
			"<!doctype html><html><body>Frontend not built. Run: yarn build</body></html>",
		)
	}

	return &SPAHandler{
		fs:    http.FileServerFS(publicFS),
		index: injectRuntimeConfig(index, cfg),
	}
}

// injectRuntimeConfig writes the frontend config into index.html as <meta> tags
// right after <head>, so one built image serves every environment (the SPA
// reads DSN + environment + the debug flag at runtime). If there is no <head>
// the document is returned unchanged and the SPA falls back to its build-time
// import.meta.env values (e.g. under the Vite dev server).
func injectRuntimeConfig(index []byte, cfg SPAConfig) []byte {
	var meta strings.Builder
	writeMeta := func(name, content string) {
		meta.WriteString(`<meta name="`)
		meta.WriteString(name)
		meta.WriteString(`" content="`)
		meta.WriteString(html.EscapeString(content))
		meta.WriteString(`">`)
	}

	writeMeta(metaSentryDSN, cfg.SentryDSN)
	writeMeta(metaSentryEnvironment, cfg.SentryEnvironment)
	if cfg.SentryDebug {
		writeMeta(metaSentryDebug, "true")
	}

	return bytes.Replace(index, []byte("<head>"), []byte("<head>"+meta.String()), 1)
}

func (h *SPAHandler) Serve(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Try serving static file first (JS, CSS, assets)
	if strings.Contains(path, ".") {
		h.fs.ServeHTTP(w, r)
		return
	}

	// SPA fallback — serve index.html for all other routes
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.index)
}
