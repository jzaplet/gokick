package handler

import (
	"html"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/presentation/http/response"
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
	resp *response.Responder
	fs   http.Handler
	// index holds the served shell per language — same bytes except the
	// <html lang> attribute, so the SPA bootstraps its locale without a
	// flash of the wrong language.
	index map[shared.Lang][]byte
}

func NewSPAHandler(
	resp *response.Responder,
	logger *slog.Logger,
	publicFS fs.FS,
	cfg SPAConfig,
) *SPAHandler {
	index, err := fs.ReadFile(publicFS, "index.html")
	if err != nil {
		// Not-built fallback: no <head>, no runtime config to inject. Dev-only
		// operator text — deliberately not localized.
		fallback := []byte(
			"<!doctype html><html><body>Frontend not built. Run: yarn build</body></html>",
		)
		return &SPAHandler{
			resp:  resp,
			fs:    http.FileServerFS(publicFS),
			index: map[shared.Lang][]byte{shared.DefaultLang: fallback},
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

	variants, ok := shellVariants(injected)
	if !ok {
		// No <html lang> anchor — serve the template as-is for every language
		// rather than failing; the SPA then falls back to its own detection.
		logger.Warn("spa: index.html has no <html lang> attribute; " +
			"serving a single-language shell")
	}

	return &SPAHandler{
		resp:  resp,
		fs:    http.FileServerFS(publicFS),
		index: variants,
	}
}

// htmlLangRe matches the lang attribute inside the first <html …> tag; the
// value is rewritten per language at construction time.
var htmlLangRe = regexp.MustCompile(`(?i)(<html\b[^>]*\blang=")[^"]*(")`)

// shellVariants pre-renders one shell per supported language by rewriting the
// <html lang> attribute. ok=false means the template exposes no lang anchor
// and every language maps to the unmodified bytes.
func shellVariants(index []byte) (map[shared.Lang][]byte, bool) {
	loc := htmlLangRe.FindSubmatchIndex(index)
	variants := make(map[shared.Lang][]byte, len(shared.SupportedLangs()))
	for _, lang := range shared.SupportedLangs() {
		if loc == nil {
			variants[lang] = index
			continue
		}
		variant := make([]byte, 0, len(index))
		variant = append(variant, index[:loc[3]]...)
		variant = append(variant, lang...)
		variant = append(variant, index[loc[4]:]...)
		variants[lang] = variant
	}
	return variants, loc != nil
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

	// An unknown /api/ path must never fall through to the SPA index — a stray or
	// mistyped API call should get a JSON 404, not a 200 text/html page it can't
	// parse (which masks the real error as a confusing "unexpected token <").
	if strings.HasPrefix(path, "/api/") {
		h.resp.HandleError(r.Context(), w,
			&shared.MessageError{Key: msgkey.CommonNotFound, Status: http.StatusNotFound})
		return
	}

	// SPA fallback — serve index.html for all other routes. The shell varies
	// per request on more than Accept-Language (the gk_lang cookie and the
	// X-App-Lang header pick the variant too), and cookie/header variance is
	// invisible to shared caches even with a Vary header — so the shell must
	// not be cached at all. Hashed static assets above stay cacheable; only
	// the HTML shell is no-store.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Accept-Language")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	//gkts:ignore static SPA shell — embedded index.html bytes, not a JSON payload
	_, _ = w.Write(h.shellFor(r))
}

// shellFor picks the shell variant: an explicit /cs|/en path prefix wins (a
// deep link into a language), otherwise the ctx language — LangMiddleware
// already resolved the full ladder (X-App-Lang → gk_lang cookie →
// Accept-Language → en) for every request, this handler included.
func (h *SPAHandler) shellFor(r *http.Request) []byte {
	lang := shared.LangFromContext(r.Context())
	if parsed, valid := shared.ParseLang(firstPathSegment(r.URL.Path)); valid {
		lang = parsed
	}
	if shell, ok := h.index[lang]; ok {
		return shell
	}

	return h.index[shared.DefaultLang]
}

// firstPathSegment returns the first path segment ("" for the root path) —
// the caller's ParseLang rejects "" along with everything else unsupported.
func firstPathSegment(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	if i := strings.IndexByte(trimmed, '/'); i >= 0 {
		trimmed = trimmed[:i]
	}

	return trimmed
}
