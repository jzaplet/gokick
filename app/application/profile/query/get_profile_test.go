package query

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func TestGetProfileHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "profile_success.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	})

	handler := NewGetProfileHandler(fx.Users)
	profile, err := handler.Handle(authCtx, GetProfileQuery{})
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}

	if profile.ID != u.ID {
		t.Fatalf("user id mismatch: got %s want %s", profile.ID, u.ID)
	}
	if profile.Nickname != "alice" {
		t.Fatalf("nickname mismatch: got %s want alice", profile.Nickname)
	}
	if profile.Email != "alice@example.com" {
		t.Fatalf("email mismatch: got %s", profile.Email)
	}
}

func TestGetProfileHandler_WithoutClaimsReturnsAuthError(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "profile_noauth.db"))

	handler := NewGetProfileHandler(fx.Users)
	_, err := handler.Handle(ctx, GetProfileQuery{})

	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestGetProfileHandler_UnknownUser(t *testing.T) {
	// Valid claims but user no longer exists in DB (deleted after token issued).
	// F-011: FindByID now returns (nil, nil); the handler maps a vanished session
	// subject to AuthError → 401 (client clears its session and re-logs), instead
	// of the pre-F-011 bogus 400 {"id":"user not found"} shown to a valid JWT.
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "profile_unknown.db"))

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: "00000000-0000-0000-0000-000000000000", Role: "user", Nickname: "ghost",
	})

	handler := NewGetProfileHandler(fx.Users)
	_, err := handler.Handle(authCtx, GetProfileQuery{})

	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError (401 for a vanished user), got %T: %v", err, err)
	}
}

func TestGetProfileQuery_RequiredPermission(t *testing.T) {
	if got := (GetProfileQuery{}).RequiredPermission(); got != "profile:read" {
		t.Fatalf("expected permission profile:read, got %q", got)
	}
}
