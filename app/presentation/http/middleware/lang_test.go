package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func TestResolveRequestLang(t *testing.T) {
	tests := []struct {
		name         string
		appLang      string
		cookieLang   string
		acceptLang   string
		want         shared.Lang
		wantExplicit bool
	}{
		{
			name:         "header wins",
			appLang:      "en",
			acceptLang:   "cs",
			want:         shared.LangEN,
			wantExplicit: true,
		},
		{
			name:         "cookie is explicit",
			cookieLang:   "cs",
			acceptLang:   "en",
			want:         shared.LangCS,
			wantExplicit: true,
		},
		{
			name:         "header beats cookie",
			appLang:      "en",
			cookieLang:   "cs",
			want:         shared.LangEN,
			wantExplicit: true,
		},
		{
			name:       "invalid cookie falls through",
			cookieLang: "de",
			acceptLang: "cs",
			want:       shared.LangCS,
		},
		{
			name:       "invalid header falls through",
			appLang:    "de",
			acceptLang: "en",
			want:       shared.LangEN,
		},
		{name: "accept language basic", acceptLang: "en", want: shared.LangEN},
		{name: "accept language region subtag", acceptLang: "cs-CZ", want: shared.LangCS},
		{name: "accept language q order", acceptLang: "en;q=0.5, cs;q=0.9", want: shared.LangCS},
		{
			name:       "accept language unsupported skipped",
			acceptLang: "de-DE, en;q=0.8",
			want:       shared.LangEN,
		},
		{
			name:       "accept language q zero rejected",
			acceptLang: "en;q=0, cs;q=0.1",
			want:       shared.LangCS,
		},
		{
			name:       "accept language malformed q rejected",
			acceptLang: "en;q=broken, cs",
			want:       shared.LangCS,
		},
		{name: "wildcard falls to the en default", acceptLang: "*", want: shared.DefaultLang},
		{name: "no signal defaults to en", want: shared.DefaultLang},
		{name: "uppercase accept tag normalized", acceptLang: "EN-us", want: shared.LangEN},
		{
			name:       "wildcard beats lower-q listed lang",
			acceptLang: "cs;q=0.2, *;q=0.9",
			want:       shared.LangEN,
		},
		{
			name:       "q zero excludes even the default lang from wildcard",
			acceptLang: "en;q=0, *",
			want:       shared.LangCS,
		},
		{
			name:       "q parameter name is case-insensitive",
			acceptLang: "en;Q=0.2, cs;q=0.9",
			want:       shared.LangCS,
		},
		{name: "q above 1 clamps to 1", acceptLang: "cs;q=5", want: shared.LangCS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.appLang != "" {
				r.Header.Set(shared.LangHeaderName, tt.appLang)
			}
			if tt.cookieLang != "" {
				r.AddCookie(&http.Cookie{Name: shared.LangCookieName, Value: tt.cookieLang})
			}
			if tt.acceptLang != "" {
				r.Header.Set("Accept-Language", tt.acceptLang)
			}
			got, explicit := ResolveRequestLang(r)
			if got != tt.want || explicit != tt.wantExplicit {
				t.Fatalf("ResolveRequestLang = (%q, %v), want (%q, %v)",
					got, explicit, tt.want, tt.wantExplicit)
			}
		})
	}
}

func TestLangMiddlewareStampsContext(t *testing.T) {
	var seen shared.Lang
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = shared.LangFromContext(r.Context())
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Language", "en-GB,en;q=0.9")
	LangMiddleware()(inner).ServeHTTP(httptest.NewRecorder(), r)

	if seen != shared.LangEN {
		t.Fatalf("ctx lang = %q, want en", seen)
	}
}

// The panic 500 body is one static {key} literal — the API ships keys, not
// prose, so no language signal may change the recovered-panic body. The chain
// mirrors production order (Lang wraps Recovery wraps Auth wraps handler) to
// keep pinning the full recovered path, with a users.lang claim AND an
// explicit header in play.
func TestPanicBodyIsStaticKey(t *testing.T) {
	jwt := testfx.NewJwt(t, 15*time.Minute)
	token, _, err := jwt.GenerateAccessToken(&shared.AuthClaims{
		UserID: "u-1", Role: "user", Nickname: "alice", Lang: "cs",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	chain := LangMiddleware()(
		RecoveryMiddleware(silentLogger(), &recordingReporter{})(
			AuthMiddleware(jwt, testResponder())(panicking)))

	const wantBody = `{"general":{"key":"common.internal_error"}}`

	t.Run("claims lang does not change the body", func(t *testing.T) {
		// No X-App-Lang, cookie, or Accept-Language — the claim is the only signal.
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, r)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		if got := rec.Body.String(); got != wantBody {
			t.Fatalf("panic body = %s, want the static keyed body", got)
		}
	})

	t.Run("explicit header does not change the body either", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(shared.LangHeaderName, "en")
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, r)

		if got := rec.Body.String(); got != wantBody {
			t.Fatalf("panic body = %s, want the static keyed body", got)
		}
	})
}
