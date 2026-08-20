package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"gokick/app/domain/shared"
)

// langExplicitKey carries one bool through ctx: whether the request chose its
// language explicitly (X-App-Lang header or gk_lang cookie). It spares
// AuthMiddleware re-running ResolveRequestLang just to learn whether the JWT's
// lang claim may still upgrade the resolved language.
type langExplicitKey struct{}

// LangMiddleware resolves the request language into ctx for every route:
// an explicit choice first (X-App-Lang header, then the gk_lang cookie),
// then Accept-Language negotiation, then the product default (en). A pure
// context-setter — it never writes a response and never fails. The API ships
// keys, not prose, so the resolved language serves the SPA shell (<html
// lang>) and future e-mails, not API bodies. AuthMiddleware later upgrades
// the value from the JWT claims when the request did not choose explicitly
// (full ladder: header → cookie → users.lang → Accept-Language → en).
func LangMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lang, explicit := ResolveRequestLang(r)
			ctx := shared.WithLang(r.Context(), lang)
			ctx = context.WithValue(ctx, langExplicitKey{}, explicit)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ResolveRequestLang returns the request language and whether it was an
// explicit client choice (a valid X-App-Lang header or gk_lang cookie)
// rather than negotiated or defaulted. AuthMiddleware uses the flag to
// decide whether the user's persisted preference may override.
func ResolveRequestLang(r *http.Request) (shared.Lang, bool) {
	if lang, ok := shared.ParseLang(r.Header.Get(shared.LangHeaderName)); ok {
		return lang, true
	}
	if cookie, err := r.Cookie(shared.LangCookieName); err == nil {
		if lang, ok := shared.ParseLang(cookie.Value); ok {
			return lang, true
		}
	}
	if lang, ok := negotiateAcceptLanguage(r.Header.Get("Accept-Language")); ok {
		return lang, false
	}

	return shared.DefaultLang, false
}

// applyClaimsLang upgrades the ctx language from the JWT's lang claim when the
// request did not choose explicitly (ladder: header → cookie → users.lang →
// Accept-Language → en). LangMiddleware wraps every route unconditionally
// (http/server), so a missing flag means no explicit choice was recorded and
// the claim may win.
//
// Reachable only on AUTHENTICATED routes, and only for a request that made no
// explicit choice — which is why the SPA sends X-App-Lang solely for a real
// switcher choice (assets/app-ui/Fetch/buildHeaders.ts): an unconditional
// header would pin explicit=true on every request and make this rung dead
// code. The SPA shell itself is served by the unauthenticated catch-all and
// carries no JWT, so it resolves without this rung by construction — see
// SPAHandler.shellFor, whose ladder deliberately omits users.lang.
func applyClaimsLang(ctx context.Context, claimsLang string) context.Context {
	if explicit, _ := ctx.Value(langExplicitKey{}).(bool); explicit {
		return ctx
	}
	lang, ok := shared.ParseLang(claimsLang)
	if !ok {
		return ctx
	}

	return shared.WithLang(ctx, lang)
}

// negotiatedLangs is the order equal q-values tie-break in: shared.DefaultLang
// first, then the remaining shared.SupportedLangs order. It derives from
// compile-time constants, so it is built once per process rather than per
// request; the negotiation q table is indexed parallel to it.
var negotiatedLangs = orderedSupportedLangs()

func orderedSupportedLangs() []shared.Lang {
	order := []shared.Lang{shared.DefaultLang}
	for _, lang := range shared.SupportedLangs() {
		if lang != shared.DefaultLang {
			order = append(order, lang)
		}
	}

	return order
}

// negotiateAcceptLanguage picks the supported language the Accept-Language
// header accepts at the highest q-value (RFC 9110 §12.5.4). Every range
// collapses to its primary subtag ("cs-CZ" and "cs" both name cs), so the rule
// per language is simply the highest q it was given; the wildcard "*" covers
// every supported language no explicit range named. q=0 marks a language as not
// acceptable. Equal q tie-breaks in negotiatedLangs order. ok=false means the
// header expressed no acceptable supported language (empty, unknown tags only,
// or everything excluded) — the caller falls back to the default. Hand-rolled
// on purpose: with a two-language catalog, primary-subtag matching is all
// browsers need, and it keeps vendor deps out of this component
// (http_middleware has no vendor grant in .go-arch-lint.yml).
func negotiateAcceptLanguage(header string) (shared.Lang, bool) {
	if strings.TrimSpace(header) == "" {
		return shared.DefaultLang, false
	}

	qs := acceptLanguageQs(header)
	best, bestQ, found := shared.DefaultLang, 0.0, false
	for i, lang := range negotiatedLangs {
		// Strict > keeps the earlier (DefaultLang-first) entry on a tie.
		if q := qs[i]; q > 0 && (!found || q > bestQ) {
			best, bestQ, found = lang, q, true
		}
	}
	if !found {
		return shared.DefaultLang, false
	}

	return best, true
}

// acceptLanguageQs folds the header into one q per entry of negotiatedLangs:
// the highest q carried by any range naming that language, or the wildcard's q
// for a language no range named. A recorded q=0 is an explicit "not acceptable"
// and must survive the wildcard fill ("en;q=0, *" excludes en), so "not named"
// is the distinct -1 sentinel rather than 0.
func acceptLanguageQs(header string) []float64 {
	qs := make([]float64, len(negotiatedLangs))
	for i := range qs {
		qs[i] = -1
	}

	wildcardQ := -1.0
	for header != "" {
		var element string
		element, header, _ = strings.Cut(header, ",")
		tag, q, ok := parseLangRange(element)
		if !ok || tag == "" {
			continue
		}
		if tag == "*" {
			wildcardQ = max(wildcardQ, q)

			continue
		}
		if i := negotiatedIndex(tag); i >= 0 {
			qs[i] = max(qs[i], q)
		}
	}

	if wildcardQ < 0 {
		return qs
	}
	for i, q := range qs {
		if q < 0 {
			qs[i] = wildcardQ
		}
	}

	return qs
}

// negotiatedIndex maps a language range to its negotiatedLangs slot by primary
// subtag ("cs-CZ" → cs), or -1 when the app ships no catalog for it.
func negotiatedIndex(tag string) int {
	primary, _, _ := strings.Cut(tag, "-")
	lang, ok := shared.ParseLang(strings.ToLower(primary))
	if !ok {
		return -1
	}
	for i, candidate := range negotiatedLangs {
		if candidate == lang {
			return i
		}
	}

	return -1
}

// parseLangRange splits one Accept-Language element ("cs-CZ;q=0.8") into its
// language range and q-value (default 1). The q parameter name is
// case-insensitive ("Q=0.8" is valid — RFC 9110 parameter names are) and the
// value is clamped to [0,1]. ok=false rejects the whole element (malformed q).
func parseLangRange(element string) (string, float64, bool) {
	tag, params, _ := strings.Cut(element, ";")
	q := 1.0
	for params != "" {
		var param string
		param, params, _ = strings.Cut(params, ";")
		param = strings.TrimSpace(param)
		if len(param) < 2 || (param[0] != 'q' && param[0] != 'Q') || param[1] != '=' {
			continue
		}
		parsed, err := strconv.ParseFloat(param[2:], 64)
		if err != nil {
			return "", 0, false
		}
		q = min(max(parsed, 0), 1)
	}

	return strings.TrimSpace(tag), q, true
}
