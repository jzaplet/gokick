package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authcmd "gokick/app/application/auth/command"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func newAuthHandler(t *testing.T) (*AuthHandler, *testfx.Fixture) {
	t.Helper()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "auth_http.db"))
	cmdBus, _, _ := fx.NewBuses()

	registry := shared.NewPermissionsRegistry([]shared.Permissioned{
		authcmd.LogoutCommand{},
	})

	h := NewAuthHandler(
		CookieSecure(false),
		cmdBus,
		authcmd.NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt),
		authcmd.NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt),
		authcmd.NewLogoutHandler(fx.Tokens),
		registry,
	)

	return h, fx
}

func doJSON(
	t *testing.T,
	handler http.HandlerFunc,
	method, path string,
	body any,
	cookies ...*http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	handler(rec, req)

	return rec
}

func findCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %q not set; got %v", name, rec.Result().Cookies())

	return nil
}

func TestAuthHandler_Login_Success(t *testing.T) {
	h, fx := newAuthHandler(t)
	fx.SeedUser(t, "alice", "secret-pwd", "user")

	rec := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"nickname": "alice",
		"password": "secret-pwd",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AccessToken == "" {
		t.Fatal("expected access_token")
	}
	if resp.AccessExpiration <= 0 {
		t.Fatalf("expected positive access_expiration, got %d", resp.AccessExpiration)
	}
	if resp.User.Nickname != "alice" {
		t.Fatalf("user.nickname: got %q want alice", resp.User.Nickname)
	}
	if resp.User.Role != "user" {
		t.Fatalf("user.role: got %q want user", resp.User.Role)
	}
	// User has auth:logout from our registry.
	if len(resp.User.Permissions) == 0 {
		t.Fatal("expected non-empty permissions for user role")
	}

	cookie := findCookie(t, rec, refreshCookieName)
	if cookie.Value == "" {
		t.Fatal("refresh cookie value is empty")
	}
	if !cookie.HttpOnly {
		t.Fatal("refresh cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie SameSite: got %v want Strict", cookie.SameSite)
	}
	if cookie.Path != refreshCookiePath {
		t.Fatalf("cookie Path: got %q want %q", cookie.Path, refreshCookiePath)
	}
}

// Login also sets a readable session-hint cookie so the SPA can skip the
// bootstrap refresh (and its 401) for guests. It must be readable (NOT
// HttpOnly), live at Path=/, and — critically — share the refresh cookie's
// expiry so it never drifts into a false negative that logs a real session out.
func TestAuthHandler_Login_SetsSessionHintCookie(t *testing.T) {
	h, fx := newAuthHandler(t)
	fx.SeedUser(t, "alice", "secret-pwd", "user")

	rec := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"nickname": "alice",
		"password": "secret-pwd",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	hint := findCookie(t, rec, sessionHintCookieName)
	if hint.Value != "1" {
		t.Fatalf("hint value: got %q want %q", hint.Value, "1")
	}
	if hint.HttpOnly {
		t.Fatal("hint cookie must be readable by JS (not HttpOnly)")
	}
	if hint.Path != "/" {
		t.Fatalf("hint Path: got %q want /", hint.Path)
	}
	// No-drift invariant: same expiry as the refresh cookie.
	refresh := findCookie(t, rec, refreshCookieName)
	if !hint.Expires.Equal(refresh.Expires) {
		t.Fatalf("hint Expires %v must equal refresh Expires %v", hint.Expires, refresh.Expires)
	}
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	h, fx := newAuthHandler(t)
	fx.SeedUser(t, "alice", "secret-pwd", "user")

	rec := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"nickname": "alice",
		"password": "wrong",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("must not set cookie on failed login")
	}
}

func TestAuthHandler_Login_MalformedJSON(t *testing.T) {
	h, _ := newAuthHandler(t)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		strings.NewReader("{not json"),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rec.Code)
	}
}

func TestAuthHandler_Refresh_WithValidCookie(t *testing.T) {
	h, fx := newAuthHandler(t)
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	rec := doJSON(t, h.Refresh, http.MethodPost, "/api/v1/auth/refresh", nil,
		&http.Cookie{Name: refreshCookieName, Value: raw},
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	cookie := findCookie(t, rec, refreshCookieName)
	if cookie.Value == raw {
		t.Fatal("refresh must rotate the cookie value")
	}
}

// Refresh (token rotation) re-sets the session-hint cookie with the SAME
// no-drift invariant as login: readable, Path=/, and matching the rotated
// refresh cookie's expiry. Only the login path was pinned before, so this guards
// the rotation path against a future divergence that would let the hint outlive
// (or predecease) the refresh cookie and log a real session out.
func TestAuthHandler_Refresh_SetsSessionHintCookie(t *testing.T) {
	h, fx := newAuthHandler(t)
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	rec := doJSON(t, h.Refresh, http.MethodPost, "/api/v1/auth/refresh", nil,
		&http.Cookie{Name: refreshCookieName, Value: raw},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	hint := findCookie(t, rec, sessionHintCookieName)
	if hint.Value != "1" {
		t.Fatalf("hint value: got %q want %q", hint.Value, "1")
	}
	if hint.HttpOnly {
		t.Fatal("hint cookie must be readable by JS (not HttpOnly)")
	}
	if hint.Path != "/" {
		t.Fatalf("hint Path: got %q want /", hint.Path)
	}
	// No-drift invariant: same expiry as the rotated refresh cookie.
	refresh := findCookie(t, rec, refreshCookieName)
	if !hint.Expires.Equal(refresh.Expires) {
		t.Fatalf("hint Expires %v must equal refresh Expires %v", hint.Expires, refresh.Expires)
	}
}

func TestAuthHandler_Refresh_MissingCookie(t *testing.T) {
	h, _ := newAuthHandler(t)

	rec := doJSON(t, h.Refresh, http.MethodPost, "/api/v1/auth/refresh", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
}

func TestAuthHandler_Refresh_InvalidCookieClearsIt(t *testing.T) {
	h, _ := newAuthHandler(t)

	rec := doJSON(t, h.Refresh, http.MethodPost, "/api/v1/auth/refresh", nil,
		&http.Cookie{Name: refreshCookieName, Value: "garbage"},
	)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
	cookie := findCookie(t, rec, refreshCookieName)
	if cookie.MaxAge >= 0 {
		t.Fatalf("invalid-refresh path must clear cookie (MaxAge=-1), got MaxAge=%d", cookie.MaxAge)
	}
}

func TestAuthHandler_Logout_RequiresAuth(t *testing.T) {
	h, _ := newAuthHandler(t)

	// No claims in context — AuthorizeMiddleware blocks with 401.
	rec := doJSON(t, h.Logout, http.MethodPost, "/api/v1/auth/logout", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_Logout_WithClaims(t *testing.T) {
	h, fx := newAuthHandler(t)
	u := fx.SeedUser(t, "alice", "pwd", "user")
	fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))
	fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))
	fx.AssertTokenCount(t, 2)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	ctx := shared.ContextWithClaims(req.Context(), &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rec.Code, rec.Body.String())
	}
	fx.AssertTokenCount(t, 0)

	cookie := findCookie(t, rec, refreshCookieName)
	if cookie.MaxAge >= 0 {
		t.Fatalf("logout must clear cookie (MaxAge=-1), got MaxAge=%d", cookie.MaxAge)
	}
}
