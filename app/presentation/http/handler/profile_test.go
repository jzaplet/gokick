package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	authcmd "gokick/app/application/auth/command"
	profilecmd "gokick/app/application/profile/command"
	profileqry "gokick/app/application/profile/query"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func newProfileHandler(t *testing.T) (*ProfileHandler, *testfx.Fixture) {
	t.Helper()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "profile_http.db"))
	cmdBus, qryBus, _ := fx.NewBuses()

	registry := shared.NewPermissionsRegistry([]shared.Permissioned{
		authcmd.LogoutCommand{},
		profilecmd.ChangePasswordCommand{},
		profileqry.GetProfileQuery{},
	})

	h := NewProfileHandler(
		cmdBus,
		qryBus,
		profileqry.NewGetProfileHandler(fx.Users),
		profilecmd.NewChangePasswordHandler(fx.Users, fx.Hasher),
		registry,
	)

	return h, fx
}

func authedReq(method, path string, body any, claims *shared.AuthClaims) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if claims != nil {
		req = req.WithContext(shared.ContextWithClaims(req.Context(), claims))
	}

	return req
}

func TestProfileHandler_Get_ReturnsUser(t *testing.T) {
	h, fx := newProfileHandler(t)
	u := fx.SeedUser(t, "alice", "pwd", "user")

	rec := httptest.NewRecorder()
	h.Get(rec, authedReq(http.MethodGet, "/api/v1/profile", nil, &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp userDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != u.ID {
		t.Fatalf("id: got %q want %q", resp.ID, u.ID)
	}
	if resp.Nickname != "alice" {
		t.Fatalf("nickname: got %q want alice", resp.Nickname)
	}
	if resp.Role != "user" {
		t.Fatalf("role: got %q want user", resp.Role)
	}
	if len(resp.Permissions) == 0 {
		t.Fatal("expected permissions populated from registry")
	}
	for _, p := range resp.Permissions {
		if strings.HasPrefix(p, "admin:") {
			t.Fatalf("user role must not have admin:* permissions, got %q", p)
		}
	}
}

func TestProfileHandler_Get_AdminGetsAdminPermissions(t *testing.T) {
	h, fx := newProfileHandler(t)
	u := fx.SeedUser(t, "root", "pwd", "admin")

	rec := httptest.NewRecorder()
	h.Get(rec, authedReq(http.MethodGet, "/api/v1/profile", nil, &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}

	var resp userDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Role != "admin" {
		t.Fatalf("role: got %q want admin", resp.Role)
	}
	// Admin registry currently has no admin:* permissions yet, but the
	// filter must behave correctly (admin gets the full registry).
	if len(resp.Permissions) == 0 {
		t.Fatal("expected non-empty permissions for admin")
	}
}

func TestProfileHandler_Get_WithoutClaims_Returns401(t *testing.T) {
	h, _ := newProfileHandler(t)

	rec := httptest.NewRecorder()
	h.Get(rec, authedReq(http.MethodGet, "/api/v1/profile", nil, nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
}

func TestProfileHandler_ChangePassword_Success(t *testing.T) {
	h, fx := newProfileHandler(t)
	u := fx.SeedUser(t, "alice", "old-pwd", "user")

	rec := httptest.NewRecorder()
	h.ChangePassword(rec, authedReq(http.MethodPut, "/api/v1/profile/password",
		map[string]string{"old_password": "old-pwd", "new_password": "new-pwd-123"},
		&shared.AuthClaims{UserID: u.ID, Role: u.Role, Nickname: u.Nickname},
	))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rec.Code, rec.Body.String())
	}

	reloaded, _ := fx.Users.FindByID(authedReq(http.MethodGet, "/", nil, nil).Context(), u.ID)
	if err := fx.Hasher.Verify("new-pwd-123", reloaded.PasswordHash); err != nil {
		t.Fatalf("new password should verify after change: %v", err)
	}
}

func TestProfileHandler_ChangePassword_WrongOldPassword(t *testing.T) {
	h, fx := newProfileHandler(t)
	u := fx.SeedUser(t, "alice", "real-pwd", "user")

	rec := httptest.NewRecorder()
	h.ChangePassword(rec, authedReq(http.MethodPut, "/api/v1/profile/password",
		map[string]string{"old_password": "wrong", "new_password": "new-pwd"},
		&shared.AuthClaims{UserID: u.ID, Role: u.Role, Nickname: u.Nickname},
	))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
}

func TestProfileHandler_ChangePassword_MalformedJSON(t *testing.T) {
	h, fx := newProfileHandler(t)
	u := fx.SeedUser(t, "alice", "pwd", "user")

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/profile/password",
		strings.NewReader("{broken"),
	)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(shared.ContextWithClaims(req.Context(), &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	}))

	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rec.Code)
	}
}

func TestProfileHandler_ChangePassword_WithoutClaims_Returns401(t *testing.T) {
	h, _ := newProfileHandler(t)

	rec := httptest.NewRecorder()
	h.ChangePassword(rec, authedReq(http.MethodPut, "/api/v1/profile/password",
		map[string]string{"old_password": "a", "new_password": "b"},
		nil,
	))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
}
