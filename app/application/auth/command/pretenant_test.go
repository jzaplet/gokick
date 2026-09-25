package command

import (
	"context"
	"errors"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// Login and refresh act before any tenant is known — the request carries no
// claims, so TenantMiddleware resolves the default tenant — while the account may
// live in any tenant. Through the production bus, a user of a non-default tenant
// under multitenancy must still log in, rotate the session and be force-logged-out
// on token reuse. On Postgres every step reaches a row the default tenant's
// row-level security would hide: the commands run on the system plane
// (shared.PreTenant), and without it this test fails there.
func TestLoginAndRefresh_UserOfAnotherTenantThroughTheBus(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t)
	acme := fx.SeedTenant(t, "Acme")
	fx.SeedUserInTenant(t, "alice", "user", acme.ID) // password123

	cmdBus, _, _ := fx.NewBuses()
	login := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	refresh := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	cmd := LoginCommand{Nickname: "alice", Password: "password123"}
	session, err := testfx.ExecCommand(ctx, cmdBus, "Login", cmd,
		func(ctx context.Context) (IssuedSession, error) { return login.Handle(ctx, cmd) })
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if session.User.TenantID != acme.ID {
		t.Fatalf("logged-in user tenant = %q, want %q", session.User.TenantID, acme.ID)
	}

	rotate := func(raw string) (IssuedSession, error) {
		c := RefreshTokenCommand{RawToken: raw}
		return testfx.ExecCommand(ctx, cmdBus, "RefreshToken", c,
			func(ctx context.Context) (IssuedSession, error) { return refresh.Handle(ctx, c) })
	}
	if _, err := rotate(session.RefreshToken); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	fx.AssertTokenCount(t, 2) // the used one + its replacement

	// Reusing the rotated token is theft: every session of the user ends.
	_, err = rotate(session.RefreshToken)
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("reuse: got %T %v, want an AuthError", err, err)
	}
	fx.AssertTokenCount(t, 0)
}
