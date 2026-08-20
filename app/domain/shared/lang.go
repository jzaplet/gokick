package shared

import "context"

// Lang is a supported UI language. The language SET is GENERATED:
// the constants, DefaultLang, SupportedLangs and ParseLang all live in
// lang_gen.go, written by `gk i18n` from the locale/messages.<lang>.json
// filenames. Adding a language is a catalog file plus `make i18n-gen`, so
// nothing in this file changes for it — only the plumbing below is
// hand-written.
type Lang string

// Wire carriers of the language choice, shared by the HTTP middleware and the
// SPA-shell handler so the names cannot drift (the FE sends/sets the same
// literals from assets/app-ui/I18n).
const (
	// LangHeaderName carries the SPA's selected UI language on every API call.
	LangHeaderName = "X-App-Lang"
	// LangCookieName is the readable explicit-choice cookie (set by the FE
	// switcher; read server-side for the shell variant and, later, SSR
	// redirects).
	LangCookieName = "gk_lang"
)

type langContextKey struct{}

// WithLang stores the resolved request language in ctx. The HTTP
// LangMiddleware sets it for every request; the run worker stamps it when
// restoring runs.lang for a handler. CLI commands set nothing and run
// locale-free on DefaultLang by design.
func WithLang(ctx context.Context, lang Lang) context.Context {
	return context.WithValue(ctx, langContextKey{}, lang)
}

// LangFromContext returns the request language, falling back to DefaultLang
// when none was resolved. Fail-open by design: language is a presentation
// concern and a missing value must never break a request (unlike tenant
// scoping, which fails closed).
func LangFromContext(ctx context.Context) Lang {
	lang, ok := ctx.Value(langContextKey{}).(Lang)
	if !ok {
		return DefaultLang
	}
	return lang
}
