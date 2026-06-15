package handler

import (
	"bytes"
	"html"
	"io/fs"
	"log/slog"
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

func NewSPAHandler(logger *slog.Logger, publicFS fs.FS, cfg SPAConfig) *SPAHandler {
	index, err := fs.ReadFile(publicFS, "index.html")
	if err != nil {
		// Not-built fallback: no <head>, no runtime config to inject.
		return &SPAHandler{
			fs: http.FileServerFS(publicFS),
			index: []byte(
				"<!doctype html><html><body>Frontend not built. Run: yarn build</body></html>",
			),
		}
	}

	injected, ok := injectRuntimeConfig(index, cfg)
	if !ok {
		// A real index.html exists but exposes no <head> anchor, so the runtime
		// config (incl. the frontend Sentry DSN) would never reach the SPA. Warn
		// loudly instead of dropping telemetry silently — but still serve the
		// page (the SPA falls back to its build-time env), so a template edit in
		// a fork can't take the whole app down.
		logger.Warn("spa: index.html has no <head> to inject runtime config into; " +
			"frontend Sentry/runtime config will be unavailable")
		injected = index
	}

	return &SPAHandler{
		fs:    http.FileServerFS(publicFS),
		index: injected,
	}
}

// injectRuntimeConfig writes the frontend config into index.html as <meta> tags
// right after the opening <head> tag, so one built image serves every
// environment (the SPA reads DSN + environment + the debug flag at runtime).
// Returns ok=false when the document has no <head> element, so the caller can
// surface it rather than dropping the config silently.
func injectRuntimeConfig(index []byte, cfg SPAConfig) ([]byte, bool) {
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

	at := headInsertPos(index)
	if at < 0 {
		return index, false
	}
	out := make([]byte, 0, len(index)+meta.Len())
	out = append(out, index[:at]...)
	out = append(out, meta.String()...)
	out = append(out, index[at:]...)

	return out, true
}

// headInsertPos returns the byte offset just after the opening <head …> tag,
// matched case-insensitively and tolerant of attributes (<head lang="cs">) and
// casing (<HEAD>) — so a routine template edit can't silently drop the injected
// config the way an exact "<head>" match would. A bare <header> is skipped (the
// byte after "<head" must end the tag name). Returns -1 when no <head> element
// is present (e.g. the not-built fallback).
func headInsertPos(index []byte) int {
	lower := bytes.ToLower(index)
	for from := 0; ; {
		i := bytes.Index(lower[from:], []byte("<head"))
		if i < 0 {
			return -1
		}
		i += from
		after := i + len("<head")
		if after >= len(index) {
			return -1
		}
		switch index[after] {
		case '>', ' ', '\t', '\n', '\r', '/':
			if j := bytes.IndexByte(index[i:], '>'); j >= 0 {
				return i + j + 1 // just past the tag's closing '>'
			}

			return -1
		}
		from = after // was <header> or similar — keep looking
	}
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
