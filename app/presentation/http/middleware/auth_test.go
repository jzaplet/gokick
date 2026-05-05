package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// captureHandler records the claims that AuthMiddleware put into the request context.
type captureHandler struct {
	claims *shared.AuthClaims
	called bool
}

func (h *captureHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	h.called = true
	h.claims = shared.ClaimsFromContext(r.Context())
}

func TestAuthMiddleware_ValidTokenSetsClaims(t *testing.T) {
	jwt := testfx.NewJwt(t, 15*time.Minute)
	token, _, err := jwt.GenerateAccessToken(&shared.AuthClaims{
		UserID: "u-1", Role: "admin", Nickname: "alice",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	capture := &captureHandler{}
	mw := AuthMiddleware(jwt)(capture)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	if !capture.called {
		t.Fatal("next handler was not called")
	}
	if capture.claims == nil {
		t.Fatal("expected claims in context")
	}
	if capture.claims.UserID != "u-1" || capture.claims.Role != "admin" ||
		capture.claims.Nickname != "alice" {
		t.Fatalf("claims mismatch: %+v", capture.claims)
	}
}

func TestAuthMiddleware_NoHeaderPassesThroughWithoutClaims(t *testing.T) {
	jwt := testfx.NewJwt(t, 15*time.Minute)

	capture := &captureHandler{}
	mw := AuthMiddleware(jwt)(capture)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	if !capture.called {
		t.Fatal("public route must still reach the handler")
	}
	if capture.claims != nil {
		t.Fatalf("expected no claims, got %+v", capture.claims)
	}
}

func TestAuthMiddleware_MissingBearerPrefixReturns401(t *testing.T) {
	jwt := testfx.NewJwt(t, 15*time.Minute)

	capture := &captureHandler{}
	mw := AuthMiddleware(jwt)(capture)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
	if capture.called {
		t.Fatal("next handler must not run on invalid auth")
	}
}

func TestAuthMiddleware_InvalidTokenReturns401(t *testing.T) {
	jwt := testfx.NewJwt(t, 15*time.Minute)

	capture := &captureHandler{}
	mw := AuthMiddleware(jwt)(capture)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.token")
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
	if capture.called {
		t.Fatal("next handler must not run on invalid token")
	}
}

func TestAuthMiddleware_ExpiredTokenReturns401(t *testing.T) {
	// Negative access expiration → token is already expired on issue.
	jwt := testfx.NewJwt(t, -1*time.Second)
	token, _, err := jwt.GenerateAccessToken(&shared.AuthClaims{UserID: "u-1"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	capture := &captureHandler{}
	mw := AuthMiddleware(jwt)(capture)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
}

func TestAuthMiddleware_EmptyBearerValueReturns401(t *testing.T) {
	jwt := testfx.NewJwt(t, 15*time.Minute)

	capture := &captureHandler{}
	mw := AuthMiddleware(jwt)(capture)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401 (empty token)", rec.Code)
	}
}
