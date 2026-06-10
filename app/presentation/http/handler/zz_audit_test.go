package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	authcmd "gokick/app/application/auth/command"
	usercmd "gokick/app/application/user/command"
	userqry "gokick/app/application/user/query"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// newAuthHandlerSecure builds an AuthHandler exactly like newAuthHandler but
// lets the test pick the CookieSecure flag, so we can pin the
// APP_COOKIE_SECURE -> cookie.Secure mapping in both directions.
func newAuthHandlerSecure(t *testing.T, secure bool) (*AuthHandler, *testfx.Fixture) {
	t.Helper()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "auth_secure_http.db"))
	cmdBus, _, _ := fx.NewBuses()

	registry := shared.NewPermissionsRegistry([]shared.Permissioned{
		authcmd.LogoutCommand{},
	})

	h := NewAuthHandler(
		CookieSecure(secure),
		cmdBus,
		authcmd.NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt),
		authcmd.NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt),
		authcmd.NewLogoutHandler(fx.Tokens),
		registry,
	)

	return h, fx
}

func newAdminUsersHandler(t *testing.T) (*AdminUsersHandler, *testfx.Fixture) {
	t.Helper()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "admin_users_http.db"))
	cmdBus, qryBus, _ := fx.NewBuses()

	h := NewAdminUsersHandler(
		cmdBus,
		qryBus,
		userqry.NewListUsersHandler(fx.Users),
		usercmd.NewCreateUserHandler(fx.Users, fx.Hasher),
		usercmd.NewUpdateUserHandler(fx.Users, fx.Hasher),
		usercmd.NewDeleteUserHandler(fx.Users),
	)

	return h, fx
}

// guide-auth-perm-07 + infra-config-wire-11: a handler built with
// CookieSecure(true) must emit the refresh cookie with Secure=true.
func TestAuthHandler_Login_CookieSecureTrueSetsSecureFlag(t *testing.T) {
	h, fx := newAuthHandlerSecure(t, true)
	fx.SeedUser(t, "alice", "secret-pwd", "user")

	rec := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"nickname": "alice",
		"password": "secret-pwd",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	cookie := findCookie(t, rec, refreshCookieName)
	if !cookie.Secure {
		t.Fatal("CookieSecure(true) must set refresh cookie Secure=true")
	}
}

// infra-config-wire-11 (paired false direction): a handler built with
// CookieSecure(false) must emit the refresh cookie with Secure=false.
func TestAuthHandler_Login_CookieSecureFalseLeavesSecureUnset(t *testing.T) {
	h, fx := newAuthHandlerSecure(t, false)
	fx.SeedUser(t, "alice", "secret-pwd", "user")

	rec := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"nickname": "alice",
		"password": "secret-pwd",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	cookie := findCookie(t, rec, refreshCookieName)
	if cookie.Secure {
		t.Fatal("CookieSecure(false) must leave refresh cookie Secure=false")
	}
}

// app-bus-28: AdminUsersHandler.Create dispatches through the bus, so the
// bus's AuthorizeMiddleware runs first. A non-admin caller hitting Create with
// a valid body must be rejected with 403 (admin:users:create denied) and no
// user must be persisted — proving the request never reached
// createUser.Handle directly.
func TestAdminUsersHandler_Create_NonAdminGets403AndDoesNotCreate(t *testing.T) {
	h, fx := newAdminUsersHandler(t)
	caller := fx.SeedUser(t, "bob", "pwd", "user")

	rec := httptest.NewRecorder()
	h.Create(rec, authedReq(http.MethodPost, "/api/v1/admin/users",
		map[string]string{
			"nickname": "victor",
			"password": "another-pwd",
			"email":    "victor@example.com",
			"role":     "user",
		},
		&shared.AuthClaims{
			UserID: caller.ID, Role: caller.Role, Nickname: caller.Nickname,
		},
	))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want 403; body=%s", rec.Code, rec.Body.String())
	}

	// The command must have been blocked before the handler ran: no user row.
	existing, err := fx.Users.FindByNickname(context.Background(), "victor")
	if err != nil {
		t.Fatalf("FindByNickname: %v", err)
	}
	if existing != nil {
		t.Fatal("non-admin Create must not persist a user (authorize blocks it)")
	}
}

// presentation-11 + overview-19: AdminUsersHandler.Create with an admin caller
// and a valid body returns 201 Created with an empty body, and the user is
// actually persisted.
func TestAdminUsersHandler_Create_AdminGets201EmptyBody(t *testing.T) {
	h, fx := newAdminUsersHandler(t)
	root := fx.SeedUser(t, "root", "pwd", "admin")

	rec := httptest.NewRecorder()
	h.Create(rec, authedReq(http.MethodPost, "/api/v1/admin/users",
		map[string]string{
			"nickname": "carol",
			"password": "carol-pwd-123",
			"email":    "carol@example.com",
			"role":     "user",
		},
		&shared.AuthClaims{UserID: root.ID, Role: root.Role, Nickname: root.Nickname},
	))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("Create success must write an empty body, got %q", rec.Body.String())
	}

	created, err := fx.Users.FindByNickname(context.Background(), "carol")
	if err != nil {
		t.Fatalf("FindByNickname: %v", err)
	}
	if created == nil {
		t.Fatal("Create with 201 must have persisted the user")
	}
	if created.Role != "user" {
		t.Fatalf("created role: got %q want user", created.Role)
	}
}

// overview-23 + presentation-12: AdminUsersHandler.List with an admin caller
// returns 200 OK with the seeded users serialized as adminUserDTO.
func TestAdminUsersHandler_List_Returns200WithSeededUsers(t *testing.T) {
	h, fx := newAdminUsersHandler(t)
	root := fx.SeedUser(t, "root", "pwd", "admin")
	fx.SeedUser(t, "alice", "pwd", "user")

	rec := httptest.NewRecorder()
	h.List(rec, authedReq(http.MethodGet, "/api/v1/admin/users", nil,
		&shared.AuthClaims{UserID: root.ID, Role: root.Role, Nickname: root.Nickname},
	))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	var dtos []adminUserDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dtos); err != nil {
		t.Fatalf("decode list body: %v", err)
	}

	byNick := make(map[string]adminUserDTO, len(dtos))
	for _, d := range dtos {
		byNick[d.Nickname] = d
	}
	for _, want := range []string{"root", "alice"} {
		d, ok := byNick[want]
		if !ok {
			t.Fatalf("list payload missing seeded user %q; got %+v", want, dtos)
		}
		if d.ID == "" {
			t.Fatalf("user %q in payload has empty id", want)
		}
	}
	if byNick["root"].Role != "admin" {
		t.Fatalf("root role in payload: got %q want admin", byNick["root"].Role)
	}
	if byNick["alice"].Role != "user" {
		t.Fatalf("alice role in payload: got %q want user", byNick["alice"].Role)
	}
}

// roadmap-84: clearRefreshCookie (used by Logout) sets MaxAge=-1 AND
// Expires=epoch (the legacy fallback). The existing logout test only checks
// MaxAge<0; here we pin the Expires=time.Unix(0,0) half too.
func TestAuthHandler_Logout_CookieCarriesEpochExpiry(t *testing.T) {
	h, fx := newAuthHandler(t)
	u := fx.SeedUser(t, "alice", "pwd", "user")

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

	cookie := findCookie(t, rec, refreshCookieName)
	if cookie.MaxAge != -1 {
		t.Fatalf("logout cookie MaxAge: got %d want -1", cookie.MaxAge)
	}
	if !cookie.Expires.Equal(time.Unix(0, 0)) {
		t.Fatalf("logout cookie Expires: got %v want epoch (1970-01-01T00:00:00Z)", cookie.Expires)
	}
}
