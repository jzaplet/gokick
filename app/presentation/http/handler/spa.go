package handler

import (
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

// headInsertPos returns the byte offset just after the opening <head …> tag, so
// the runtime config can be injected as its first children. It tolerates the
// realistic ways a Vite-built template writes the head — case (<HEAD>),
// attributes (<head lang="cs">), surrounding whitespace, and a '>' inside a
// quoted attribute value — and distinguishes <head> from <header>. It does NOT
// parse HTML: a literal "<head" inside a leading comment would be matched, and
// the <meta> tags would then be injected inside that comment and ignored by the
// browser (degrading to the build-time config rather than crashing). A real Vite
// index.html emits a clean <head> as an early element, so byte-scanning is
// sufficient here; a full HTML parser would be over-engineering for this anchor.
// Returns -1 when there is no <head> element (e.g. the not-built fallback),
// which the caller surfaces as a warning rather than a silent no-op.
func headInsertPos(index []byte) int {
	for i := 0; i < len(index); i++ {
		if isHeadOpenTag(index[i:]) {
			return tagClose(index, i+len("<head")) // just past the tag's '>'
		}
	}

	return -1
}

// isHeadOpenTag reports whether s starts with an opening <head> tag: "<head"
// (ASCII case-insensitive, compared WITHOUT lowercasing the document so byte
// offsets never drift on a length-changing rune) followed by a tag-name
// boundary, which distinguishes <head> / <head …> from <header>.
func isHeadOpenTag(s []byte) bool {
	const tag = "<head"
	if len(s) <= len(tag) {
		return false
	}
	for i := 0; i < len(tag); i++ {
		if toLowerASCII(s[i]) != tag[i] {
			return false
		}
	}
	switch s[len(tag)] {
	case '>', ' ', '\t', '\n', '\r', '/':
		return true
	}

	return false
}

// tagClose returns the offset just past the '>' that closes a tag whose
// attribute region starts at `from`, skipping any '>' inside single- or
// double-quoted attribute values. Returns -1 if the tag is never closed.
func tagClose(index []byte, from int) int {
	var quote byte
	for j := from; j < len(index); j++ {
		switch c := index[j]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j + 1
		}
	}

	return -1
}

func toLowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}

	return b
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
