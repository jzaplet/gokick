package postgres_test

import (
	"context"
	"strings"
	"testing"

	"gokick/app/internal/testfx/pgfx"
)

// The gokick roles pass the startup check.
func TestVerifyRoles_AcceptsTheGokickRoles(t *testing.T) {
	_, mgr := migrated(t)
	if err := mgr.VerifyRoles(context.Background()); err != nil {
		t.Fatalf("VerifyRoles: %v", err)
	}
}

// Every DSN mix-up that would quietly defeat row-level security is refused —
// the owner, a superuser or the BYPASSRLS role on the tenant plane, and a role
// without BYPASSRLS (or a superuser) on the system plane.
func TestVerifyRoles_RefusesRolesThatDefeatTheTenantWall(t *testing.T) {
	db := pgfx.New(t)
	for _, tc := range []struct {
		name              string
		appURL, systemURL string
		want              string
	}{
		{"owner as the tenant plane", db.OwnerURL, db.SystemURL, "owns"},
		{"superuser as the tenant plane", db.AdminURL, db.SystemURL, "superuser"},
		{"system role as the tenant plane", db.SystemURL, db.SystemURL, "BYPASSRLS"},
		{"tenant role as the system plane", db.AppURL, db.AppURL, "lacks BYPASSRLS"},
		{"owner as the system plane", db.AppURL, db.OwnerURL, "lacks BYPASSRLS"},
		{"superuser as the system plane", db.AppURL, db.AdminURL, "superuser"},
	} {
		cfg := db.Config()
		cfg.DBURL, cfg.DBSystemURL = tc.appURL, tc.systemURL
		err := newManager(t, cfg).VerifyRoles(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error mentioning %q", tc.name, err, tc.want)
		}
	}
}
